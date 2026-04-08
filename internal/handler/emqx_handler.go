package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go-mqtt/internal/repository"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

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

const (
	coreEventClientConnected    = "client.connected"
	coreEventClientDisconnected = "client.disconnected"
)

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
	if !isWebhookIPAllowed(c.ClientIP()) {
		c.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "message": "forbidden source ip", "data": nil})
		return
	}

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "read request body failed", "data": nil})
		return
	}

	if !verifyWebhookSignature(c.GetHeader(getWebhookSignHeaderName()), rawBody) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "message": "invalid webhook signature", "data": nil})
		return
	}

	var req emqxWebhookRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "invalid webhook payload", "data": nil})
		return
	}

	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "message": "username is required", "data": nil})
		return
	}

	status, ok := parseOnlineStatus(req.Event, req.Action)
	if !ok {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "ignored event",
			"data": gin.H{
				"supported_events": []string{coreEventClientConnected, coreEventClientDisconnected},
				"event":            req.Event,
				"action":           req.Action,
			},
		})
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
	case coreEventClientConnected:
		return 1, true
	case coreEventClientDisconnected:
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

func getWebhookSignHeaderName() string {
	v := strings.TrimSpace(os.Getenv("EMQX_WEBHOOK_SIGN_HEADER"))
	if v == "" {
		return "X-EMQX-Signature"
	}
	return v
}

func isWebhookIPAllowed(clientIP string) bool {
	raw := strings.TrimSpace(os.Getenv("EMQX_WEBHOOK_IP_WHITELIST"))
	if raw == "" {
		return true
	}

	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return false
	}

	items := strings.Split(raw, ",")
	for _, item := range items {
		entry := strings.TrimSpace(item)
		if entry == "" {
			continue
		}

		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err == nil && cidr.Contains(ip) {
				return true
			}
			continue
		}

		allowIP := net.ParseIP(entry)
		if allowIP != nil && allowIP.Equal(ip) {
			return true
		}
	}

	return false
}

func verifyWebhookSignature(signatureHeader string, body []byte) bool {
	secret := strings.TrimSpace(os.Getenv("EMQX_WEBHOOK_HMAC_SECRET"))
	if secret == "" {
		return true
	}

	got, ok := parseSignatureHeader(signatureHeader)
	if !ok {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)

	return hmac.Equal(got, expected)
}

func parseSignatureHeader(signatureHeader string) ([]byte, bool) {
	v := strings.TrimSpace(signatureHeader)
	if v == "" {
		return nil, false
	}
	v = strings.TrimPrefix(strings.ToLower(v), "sha256=")
	b, err := hex.DecodeString(v)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}
