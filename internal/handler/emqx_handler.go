package handler

import (
	"errors"
	"go-mqtt/internal/repository"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EMQX鉴权与Webhook处理
type EMQXHandler struct {
	Repo *repository.DeviceRepository
}

func NewEMQXHandler() *EMQXHandler {
	return &EMQXHandler{Repo: repository.NewDeviceRepository()}
}

type emqxAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	ClientID string `json:"clientid"`
}

type emqxWebhookRequest struct {
	Event    string `json:"event"`
	Action   string `json:"action"`
	Username string `json:"username"`
	ClientID string `json:"clientid"`
}

// Auth 提供给EMQX HTTP认证使用
func (h *EMQXHandler) Auth(c *gin.Context) {
	var req emqxAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"result": "deny", "is_superuser": false})
		return
	}

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusOK, gin.H{"result": "deny", "is_superuser": false})
		return
	}

	// 允许配置一个Neuron网关专用账号，便于网关统一接入EMQX
	if allowNeuronGateway(req.Username, req.Password) {
		c.JSON(http.StatusOK, gin.H{"result": "allow", "is_superuser": false})
		return
	}

	device, err := h.Repo.GetByDeviceID(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{"result": "deny", "is_superuser": false})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "auth query failed"})
		return
	}

	if device.DeviceSecret != req.Password {
		c.JSON(http.StatusOK, gin.H{"result": "deny", "is_superuser": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"result": "allow", "is_superuser": false})
}

func allowNeuronGateway(username, password string) bool {
	gwUser := os.Getenv("NEURON_GATEWAY_USERNAME")
	gwPass := os.Getenv("NEURON_GATEWAY_PASSWORD")
	if gwUser == "" || gwPass == "" {
		return false
	}
	return username == gwUser && password == gwPass
}

// Webhook 处理EMQX上下线事件并回写设备状态
func (h *EMQXHandler) Webhook(c *gin.Context) {
	var req emqxWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "invalid webhook payload", "data": nil})
		return
	}

	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "username is required", "data": nil})
		return
	}

	status, ok := parseOnlineStatus(req.Event, req.Action)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ignored event", "data": nil})
		return
	}

	if err := h.Repo.UpdateStatusByDeviceID(req.Username, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "update device status failed", "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{}})
}

func parseOnlineStatus(event, action string) (int, bool) {
	switch event {
	case "client.connected":
		return 1, true
	case "client.disconnected":
		return 0, true
	}

	switch action {
	case "connected":
		return 1, true
	case "disconnected":
		return 0, true
	}

	return 0, false
}
