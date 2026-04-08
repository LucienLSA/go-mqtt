package handler

import (
	"encoding/json"
	"go-mqtt/internal/model"
	"go-mqtt/internal/mqtt"
	"go-mqtt/internal/repository"
	"go-mqtt/internal/response"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 设备方法定义
type DeviceHandler struct {
	Repo    *repository.DeviceRepository
	CmdRepo *repository.CommandLogRepository
}

// 创建对象
func NewDeviceHandler() *DeviceHandler {
	return &DeviceHandler{
		Repo:    repository.NewDeviceRepository(),
		CmdRepo: repository.NewCommandLogRepository(),
	}
}

// 生成DeviceID，可改进使用UUID或者分布式ID
func generateDeviceID() string {
	return "DEV" + strconv.FormatInt(time.Now().Unix(), 10) + strconv.Itoa(rand.Intn(1000))
}

// 生成DeviceSecret，可改进使用加密算法的第三方库
func generateDeviceSecret() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// 创建设备
func (h *DeviceHandler) CreateDevice(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		GroupID uint   `json:"group_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	device := &model.Device{
		DeviceID:     generateDeviceID(),
		DeviceSecret: generateDeviceSecret(),
		Name:         req.Name,
		GroupID:      req.GroupID,
		Status:       0,
	}

	if err := h.Repo.Create(device); err != nil {
		response.Fail(c, http.StatusInternalServerError, "创建设备失败")
		return
	}
	response.Success(c, device)
}

// 获取设备详情
func (h *DeviceHandler) GetDevice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "无效的ID")
		return
	}

	device, err := h.Repo.GetByID(uint(id))
	if err != nil {
		response.Fail(c, http.StatusNotFound, "设备不存在")
		return
	}
	response.Success(c, device)
}

// 获取设备列表
func (h *DeviceHandler) GetDeviceList(c *gin.Context) {
	devices, err := h.Repo.GetAll()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "获取设备列表失败")
		return
	}
	response.Success(c, devices)
}

// 更新设备
func (h *DeviceHandler) UpdateDevice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "无效的ID")
		return
	}

	device, err := h.Repo.GetByID(uint(id))
	if err != nil {
		response.Fail(c, http.StatusNotFound, "设备不存在")
		return
	}

	var req struct {
		Name    string `json:"name"`
		GroupID *uint  `json:"group_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name != "" {
		device.Name = req.Name
	}
	if req.GroupID != nil {
		device.GroupID = *req.GroupID
	}

	if err := h.Repo.Update(device); err != nil {
		response.Fail(c, http.StatusInternalServerError, "更新设备失败")
		return
	}
	response.Success(c, device)
}

// 删除设备
func (h *DeviceHandler) DeleteDevice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "无效的ID")
		return
	}

	if err := h.Repo.Delete(uint(id)); err != nil {
		response.Fail(c, http.StatusInternalServerError, "删除设备失败")
		return
	}
	response.Success(c, gin.H{})
}

// 下发控制指令
func (h *DeviceHandler) ControlDevice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "无效的ID")
		return
	}

	device, err := h.Repo.GetByID(uint(id))
	if err != nil {
		response.Fail(c, http.StatusNotFound, "设备不存在")
		return
	}

	var req struct {
		Cmd   string      `json:"cmd" binding:"required"`
		Param interface{} `json:"param"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	traceID := generateTraceID()
	topic := "device/" + device.DeviceID + "/control"
	ackTimeout := envInt("CMD_ACK_TIMEOUT_SEC", 15)
	maxRetry := envInt("CMD_MAX_RETRY", 2)
	now := time.Now().Unix()
	timeoutAt := now + int64(ackTimeout)
	payload := gin.H{
		"cmd":      req.Cmd,
		"param":    req.Param,
		"trace_id": traceID,
		"ts":       now,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "指令序列化失败")
		return
	}

	cmdLog := &model.CommandLog{
		DeviceID:    device.DeviceID,
		TraceID:     traceID,
		Topic:       topic,
		Payload:     string(body),
		Command:     req.Cmd,
		Result:      "pending",
		Status:      0,
		RetryCount:  0,
		MaxRetry:    maxRetry,
		TimeoutAt:   timeoutAt,
		NextRetryAt: timeoutAt,
		Message:     "issued",
	}
	if err := h.CmdRepo.Create(cmdLog); err != nil {
		response.Fail(c, http.StatusInternalServerError, "命令日志写入失败")
		return
	}

	sub := mqtt.DefaultSubscriber()
	if sub == nil {
		response.Fail(c, http.StatusServiceUnavailable, "MQTT订阅器未初始化")
		return
	}

	if err := sub.PublishControl(device.DeviceID, body); err != nil {
		response.Fail(c, http.StatusInternalServerError, "MQTT下发失败")
		return
	}
	response.Success(c, gin.H{
		"device_id":  device.DeviceID,
		"trace_id":   traceID,
		"topic":      topic,
		"timeout_at": timeoutAt,
		"max_retry":  maxRetry,
	})
}

// 查询命令历史
func (h *DeviceHandler) GetCommandHistory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "无效的ID")
		return
	}

	device, err := h.Repo.GetByID(uint(id))
	if err != nil {
		response.Fail(c, http.StatusNotFound, "设备不存在")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	logs, err := h.CmdRepo.ListByDeviceID(device.DeviceID, limit)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "查询命令历史失败")
		return
	}
	response.Success(c, logs)
}

func generateTraceID() string {
	return "ctrl-" + time.Now().Format("20060102150405") + "-" + strconv.Itoa(rand.Intn(1000000))
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}
