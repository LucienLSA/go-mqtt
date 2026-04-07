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
type Subscriber struct {
	client     mqtt.Client
	deviceRepo *repository.DeviceRepository
	dataRepo   *repository.SensorDataRepository
	cmdRepo    *repository.CommandLogRepository
}

var defaultSubscriber *Subscriber

func NewSubscriber() *Subscriber {
	return &Subscriber{
		deviceRepo: repository.NewDeviceRepository(),
		dataRepo:   repository.NewSensorDataRepository(),
		cmdRepo:    repository.NewCommandLogRepository(),
	}
}

func SetDefaultSubscriber(s *Subscriber) {
	defaultSubscriber = s
}

func DefaultSubscriber() *Subscriber {
	return defaultSubscriber
}

func (s *Subscriber) Start() error {
	broker := getEnv("MQTT_BROKER", "tcp://127.0.0.1:1883")
	dataTopic := getEnv("MQTT_TOPIC_DATA", "device/+/data")
	feedbackTopic := getEnv("MQTT_TOPIC_FEEDBACK", "device/+/feedback")
	qos := byte(0)
	if v, err := strconv.Atoi(getEnv("MQTT_QOS", "0")); err == nil && v >= 0 && v <= 2 {
		qos = byte(v)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(getEnv("MQTT_CLIENT_ID", "go-mqtt-subscriber-"+strconv.FormatInt(time.Now().UnixNano(), 10)))
	opts.SetUsername(os.Getenv("MQTT_USERNAME"))
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		tokenData := c.Subscribe(dataTopic, qos, s.handleDataMessage)
		if tokenData.Wait() && tokenData.Error() != nil {
			log.Printf("MQTT订阅失败 topic=%s err=%v", dataTopic, tokenData.Error())
			return
		}
		log.Printf("MQTT订阅成功 topic=%s qos=%d", dataTopic, qos)

		tokenFeedback := c.Subscribe(feedbackTopic, qos, s.handleFeedbackMessage)
		if tokenFeedback.Wait() && tokenFeedback.Error() != nil {
			log.Printf("MQTT订阅失败 topic=%s err=%v", feedbackTopic, tokenFeedback.Error())
			return
		}
		log.Printf("MQTT订阅成功 topic=%s qos=%d", feedbackTopic, qos)
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Printf("MQTT连接断开 err=%v", err)
	})

	s.client = mqtt.NewClient(opts)
	token := s.client.Connect()
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	log.Printf("MQTT连接成功 broker=%s", broker)
	return nil
}

func (s *Subscriber) PublishControl(deviceID string, payload []byte) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("mqtt client is not initialized")
	}
	if !s.client.IsConnected() {
		return fmt.Errorf("mqtt client is disconnected")
	}
	topic := fmt.Sprintf("device/%s/control", deviceID)
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

func (s *Subscriber) handleDataMessage(_ mqtt.Client, msg mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("MQTT消息处理异常 topic=%s panic=%v", msg.Topic(), r)
		}
	}()

	deviceID, ok := parseDeviceID(msg.Topic())
	if !ok {
		log.Printf("忽略非法topic topic=%s", msg.Topic())
		return
	}

	if _, err := s.deviceRepo.GetByDeviceID(deviceID); err != nil {
		log.Printf("未知设备上报 device_id=%s topic=%s", deviceID, msg.Topic())
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		log.Printf("解析上报失败 device_id=%s topic=%s err=%v", deviceID, msg.Topic(), err)
		return
	}

	data := &model.SensorData{
		DeviceID: deviceID,
		Temp:     toFloat64(payload["temp"]),
		Humi:     toFloat64(payload["humi"]),
		Voltage:  toFloat64(payload["voltage"]),
		Status:   toStatus(payload["status"]),
		Ts:       toInt64OrNow(payload["ts"]),
	}

	if err := s.dataRepo.Create(data); err != nil {
		log.Printf("上报落库失败 device_id=%s err=%v", deviceID, err)
		return
	}

	_ = s.deviceRepo.UpdateStatusByDeviceID(deviceID, 1)
	log.Printf("上报落库成功 device_id=%s topic=%s", deviceID, msg.Topic())
}

func (s *Subscriber) handleFeedbackMessage(_ mqtt.Client, msg mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("MQTT回执处理异常 topic=%s panic=%v", msg.Topic(), r)
		}
	}()

	var payload struct {
		TraceID string `json:"trace_id"`
		Result  string `json:"result"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		log.Printf("解析回执失败 topic=%s err=%v", msg.Topic(), err)
		return
	}

	if payload.TraceID == "" {
		log.Printf("忽略无trace_id回执 topic=%s", msg.Topic())
		return
	}

	status := 2
	if strings.EqualFold(payload.Result, "ok") || strings.EqualFold(payload.Result, "success") {
		status = 1
	}

	if err := s.cmdRepo.UpdateByTraceID(payload.TraceID, payload.Result, payload.Message, status); err != nil {
		log.Printf("更新命令回执失败 trace_id=%s err=%v", payload.TraceID, err)
		return
	}

	log.Printf("命令回执更新成功 trace_id=%s result=%s", payload.TraceID, payload.Result)
}

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
