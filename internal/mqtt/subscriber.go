package mqtt

import (
	"encoding/json"
	"fmt"
	"go-mqtt/internal/model"
	"go-mqtt/internal/repository"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Subscriber 负责订阅设备上报并落库
// 负责：订阅设备上报消息/回执消息、消息落库、下发控制命令、命令重试
type Subscriber struct {
	client     mqtt.Client
	deviceRepo *repository.DeviceRepository
	dataRepo   *repository.SensorDataRepository
	cmdRepo    *repository.CommandLogRepository
}

// defaultSubscriber 全局默认订阅器单例
var defaultSubscriber *Subscriber

// NewSubscriber 创建订阅器实例
// 初始化依赖的所有仓储层对象
func NewSubscriber() *Subscriber {
	return &Subscriber{
		deviceRepo: repository.NewDeviceRepository(),
		dataRepo:   repository.NewSensorDataRepository(),
		cmdRepo:    repository.NewCommandLogRepository(),
	}
}

// SetDefaultSubscriber 设置全局默认订阅器
func SetDefaultSubscriber(s *Subscriber) {
	defaultSubscriber = s
}

// DefaultSubscriber 获取全局默认订阅器
func DefaultSubscriber() *Subscriber {
	return defaultSubscriber
}

// Start 启动MQTT订阅器
// 1. 加载环境变量配置
// 2. 创建并连接MQTT客户端
// 3. 订阅设备数据主题和命令回执主题
// 4. 启动命令重试协程
func (s *Subscriber) Start() error {
	// 从环境变量读取MQTT配置，无配置则使用默认值
	broker := getEnv("MQTT_BROKER", "tcp://127.0.0.1:1883")
	// 设备上报数据主题（通配符+匹配任意设备ID）
	dataTopic := getEnv("MQTT_TOPIC_DATA", "device/+/data")
	// 设备命令回执主题
	feedbackTopic := getEnv("MQTT_TOPIC_FEEDBACK", "device/+/feedback")
	// MQTT服务质量 0-最多一次 1-至少一次 2-仅一次
	qos := byte(0)
	if v, err := strconv.Atoi(getEnv("MQTT_QOS", "0")); err == nil && v >= 0 && v <= 2 {
		qos = byte(v)
	}

	// 创建MQTT客户端配置选项
	opts := mqtt.NewClientOptions()
	// 设置MQTT Broker地址
	opts.AddBroker(broker)
	// 设置客户端ID（默认生成唯一ID，防止重复，使用时间戳）
	opts.SetClientID(getEnv("MQTT_CLIENT_ID", "go-mqtt-subscriber-"+strconv.FormatInt(time.Now().UnixNano(), 10)))
	// MQTT用户名
	opts.SetUsername(os.Getenv("MQTT_USERNAME"))
	// MQTT密码
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	// 开启自动重连
	opts.SetAutoReconnect(true)
	// MQTT连接成功回调：连接成功后订阅主题
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		// 订阅设备上报数据主题
		tokenData := c.Subscribe(dataTopic, qos, s.handleDataMessage)
		if tokenData.Wait() && tokenData.Error() != nil {
			log.Printf("MQTT订阅失败 topic=%s err=%v", dataTopic, tokenData.Error())
			return
		}
		log.Printf("MQTT订阅成功 topic=%s qos=%d", dataTopic, qos)

		// 订阅设备命令回执主题
		tokenFeedback := c.Subscribe(feedbackTopic, qos, s.handleFeedbackMessage)
		if tokenFeedback.Wait() && tokenFeedback.Error() != nil {
			log.Printf("MQTT订阅失败 topic=%s err=%v", feedbackTopic, tokenFeedback.Error())
			return
		}
		log.Printf("MQTT订阅成功 topic=%s qos=%d", feedbackTopic, qos)
	})
	// MQTT连接断开回调
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Printf("MQTT连接断开 err=%v", err)
	})

	// 创建MQTT客户端并连接
	s.client = mqtt.NewClient(opts)
	token := s.client.Connect()
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	log.Printf("MQTT连接成功 broker=%s", broker)

	// 启动命令重试定时任务
	s.startCommandRetryWorker()
	return nil
}

// PublishControl 下发设备控制命令
// deviceID: 设备ID  payload: 命令JSON字节数组
func (s *Subscriber) PublishControl(deviceID string, payload []byte) error {
	// 校验客户端状态
	if s == nil || s.client == nil {
		return fmt.Errorf("mqtt client is not initialized")
	}
	if !s.client.IsConnected() {
		return fmt.Errorf("mqtt client is disconnected")
	}
	// 拼接控制命令主题：device/{设备ID}/control
	topic := fmt.Sprintf("device/%s/control", deviceID)
	// 获取QoS配置
	qos := byte(0)
	if v, err := strconv.Atoi(getEnv("MQTT_QOS", "0")); err == nil && v >= 0 && v <= 2 {
		qos = byte(v)
	}
	// 发布消息（非保留消息）
	token := s.client.Publish(topic, qos, false, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// handleDataMessage 处理【设备上报数据】的MQTT消息
func (s *Subscriber) handleDataMessage(_ mqtt.Client, msg mqtt.Message) {
	// 捕获panic，防止单个消息处理异常导致程序崩溃
	defer func() {
		if r := recover(); r != nil {
			log.Printf("MQTT消息处理异常 topic=%s panic=%v", msg.Topic(), r)
		}
	}()

	// 从Topic中解析设备ID：格式 device/xxx/data
	deviceID, ok := parseDeviceID(msg.Topic())
	if !ok {
		log.Printf("忽略非法topic topic=%s", msg.Topic())
		return
	}

	// 校验设备是否存在（不存在则忽略上报）
	if _, err := s.deviceRepo.GetByDeviceID(deviceID); err != nil {
		log.Printf("未知设备上报 device_id=%s topic=%s", deviceID, msg.Topic())
		return
	}

	// 解析JSON上报数据
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		log.Printf("解析上报失败 device_id=%s topic=%s err=%v", deviceID, msg.Topic(), err)
		return
	}

	// 构造传感器数据模型
	data := &model.SensorData{
		DeviceID: deviceID,
		Temp:     toFloat64(payload["temp"]),
		Humi:     toFloat64(payload["humi"]),
		Voltage:  toFloat64(payload["voltage"]),
		Status:   toStatus(payload["status"]),
		Ts:       toInt64OrNow(payload["ts"]),
	}

	// 数据落库
	if err := s.dataRepo.Create(data); err != nil {
		log.Printf("上报落库失败 device_id=%s err=%v", deviceID, err)
		return
	}

	// 更新设备在线状态为在线(status=1)
	_ = s.deviceRepo.UpdateStatusByDeviceID(deviceID, 1)
	log.Printf("上报落库成功 device_id=%s topic=%s", deviceID, msg.Topic())
}

// handleFeedbackMessage 处理【设备命令回执】的MQTT消息
// 回调函数：订阅到回执主题后自动执行
func (s *Subscriber) handleFeedbackMessage(_ mqtt.Client, msg mqtt.Message) {
	// 捕获panic，防止异常崩溃
	defer func() {
		if r := recover(); r != nil {
			log.Printf("MQTT回执处理异常 topic=%s panic=%v", msg.Topic(), r)
		}
	}()

	// 定义回执消息结构体
	var payload struct {
		TraceID string `json:"trace_id"`
		Result  string `json:"result"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		log.Printf("解析回执失败 topic=%s err=%v", msg.Topic(), err)
		return
	}

	// 无TraceID则忽略（无法匹配命令）
	if payload.TraceID == "" {
		log.Printf("忽略无trace_id回执 topic=%s", msg.Topic())
		return
	}

	// 判定命令执行状态：1=成功 2=失败
	status := 2
	if strings.EqualFold(payload.Result, "ok") || strings.EqualFold(payload.Result, "success") {
		status = 1
	}

	// 更新命令日志的回执信息
	updated, err := s.cmdRepo.UpdateByTraceID(payload.TraceID, payload.Result, payload.Message, status)
	if err != nil {
		log.Printf("更新命令回执失败 trace_id=%s err=%v", payload.TraceID, err)
		return
	}

	// 未更新成功：说明是重复回执，忽略
	if !updated {
		log.Printf("忽略重复回执 trace_id=%s", payload.TraceID)
		return
	}

	log.Printf("命令回执更新成功 trace_id=%s result=%s", payload.TraceID, payload.Result)
}

// startCommandRetryWorker 启动【命令重试定时任务】
// 定时扫描超时未回执的命令，进行重发
func (s *Subscriber) startCommandRetryWorker() {
	scanInterval := envInt("CMD_SCAN_INTERVAL_SEC", 2)
	ackTimeout := envInt("CMD_ACK_TIMEOUT_SEC", 15)
	retryInterval := envInt("CMD_RETRY_INTERVAL_SEC", 5)

	// 开启协程执行定时任务
	go func() {
		ticker := time.NewTicker(time.Duration(scanInterval) * time.Second)
		defer ticker.Stop()

		// 循环执行：每隔scanInterval秒处理一次待重试命令
		for range ticker.C {
			s.processPendingCommands(ackTimeout, retryInterval)
		}
	}()
}

// processPendingCommands 处理【待重试/超时】的控制命令
func (s *Subscriber) processPendingCommands(ackTimeout, retryInterval int) {
	now := time.Now().Unix()
	// 查询待重试的命令（最多200条，防止压力过大）
	logs, err := s.cmdRepo.ListPendingForRetry(now, 200)
	if err != nil {
		log.Printf("命令重试扫描失败 err=%v", err)
		return
	}

	// 遍历处理每条待重试命令
	for _, cmd := range logs {
		if cmd.RetryCount >= cmd.MaxRetry {
			_ = s.cmdRepo.MarkTimeout(cmd.TraceID, "ack timeout exceeded max retries")
			log.Printf("命令超时 trace_id=%s retry=%d/%d", cmd.TraceID, cmd.RetryCount, cmd.MaxRetry)
			continue
		}

		// 下一次重试次数
		nextRetryCount := cmd.RetryCount + 1
		// 拼接命令主题（无则使用默认主题）
		topic := cmd.Topic
		if topic == "" {
			topic = fmt.Sprintf("device/%s/control", cmd.DeviceID)
		}

		// 重发命令
		publishErr := s.publishRaw(topic, []byte(cmd.Payload))
		// 计算下一次重试时间、超时时间
		nextAt := now + int64(retryInterval)
		timeoutAt := now + int64(ackTimeout)

		// 构造日志信息
		msg := fmt.Sprintf("retry %d/%d sent", nextRetryCount, cmd.MaxRetry)
		if publishErr != nil {
			msg = fmt.Sprintf("retry %d/%d publish failed: %v", nextRetryCount, cmd.MaxRetry, publishErr)
		}

		// 更新命令的重试计划（次数、时间、日志）
		if err := s.cmdRepo.UpdateRetryPlan(cmd.TraceID, nextRetryCount, timeoutAt, nextAt, msg); err != nil {
			log.Printf("更新命令重试计划失败 trace_id=%s err=%v", cmd.TraceID, err)
			continue
		}

		// 命令重发失败日志
		if publishErr != nil {
			log.Printf("命令重发失败 trace_id=%s err=%v", cmd.TraceID, publishErr)
			continue
		}
		log.Printf("命令重发成功 trace_id=%s retry=%d/%d", cmd.TraceID, nextRetryCount, cmd.MaxRetry)
	}
}

// publishRaw 底层消息发布方法（封装通用发布逻辑）
func (s *Subscriber) publishRaw(topic string, payload []byte) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("mqtt client is not initialized")
	}
	if !s.client.IsConnected() {
		return fmt.Errorf("mqtt client is disconnected")
	}
	qos := byte(0)
	if v, err := strconv.Atoi(getEnv("MQTT_QOS", "0")); err == nil && v >= 0 && v <= 2 {
		qos = byte(v)
	}
	token := s.client.Publish(topic, qos, false, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// parseDeviceID 从Topic解析设备ID
// 合法Topic格式：device/xxx/data → 返回xxx
func parseDeviceID(topic string) (string, bool) {
	parts := strings.Split(topic, "/")
	if len(parts) != 3 {
		return "", false
	}
	if parts[0] != "device" || parts[2] != "data" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

func toInt64OrNow(v any) int64 {
	now := time.Now().Unix()
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return now
		}
		return i
	case string:
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return now
		}
		return i
	default:
		return now
	}
}

func toStatus(v any) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case string:
		s := strings.ToLower(strings.TrimSpace(val))
		if s == "running" || s == "online" || s == "ok" || s == "1" {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
