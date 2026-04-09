package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"go-mqtt/internal/auth"
	"go-mqtt/internal/model"
	"go-mqtt/internal/repository"
	"go-mqtt/internal/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct {
	Repo *repository.AuthUserRepository
}

func NewAuthHandler() *AuthHandler {
	h := &AuthHandler{Repo: repository.NewAuthUserRepository()}
	h.bootstrapUsers()
	return h
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	u, err := h.Repo.GetByUsername(strings.TrimSpace(req.Username))
	if err != nil || u.Status != 1 || !auth.VerifyPassword(u.PasswordHash, req.Password) {
		response.Fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	token, exp, err := auth.GenerateToken(u.Username, u.Role)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "token生成失败")
		return
	}

	response.Success(c, gin.H{
		"token":      token,
		"role":       u.Role,
		"expires_at": exp,
	})
}

// bootstrapUsers 从环境变量加载初始用户并创建（如果不存在）
func (h *AuthHandler) bootstrapUsers() {
	// 从环境变量加载用户列表
	seed := auth.LoadUsersFromEnv()
	for _, item := range seed {
		_, err := h.Repo.GetByUsername(item.Username)
		if err == nil {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("query auth user failed username=%s err=%v", item.Username, err)
			continue
		}
		// 用户不存在，创建新用户
		hash, hashErr := auth.HashPassword(item.Password)
		if hashErr != nil {
			log.Printf("hash auth user password failed username=%s err=%v", item.Username, hashErr)
			continue
		}

		user := &model.AuthUser{
			Username:     item.Username,
			PasswordHash: hash,
			Role:         strings.ToLower(strings.TrimSpace(item.Role)),
			Status:       1,
		}
		if createErr := h.Repo.Create(user); createErr != nil {
			log.Printf("create auth user failed username=%s err=%v", item.Username, createErr)
			continue
		}
		log.Printf("auth user bootstrapped username=%s role=%s", user.Username, user.Role)
	}
}

func (h *AuthHandler) Me(c *gin.Context) {
	username, _ := c.Get("auth_username")
	role, _ := c.Get("auth_role")
	response.Success(c, gin.H{
		"username": username,
		"role":     role,
	})
}
