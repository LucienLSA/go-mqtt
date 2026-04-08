package middleware

import (
	"net/http"
	"strings"

	"go-mqtt/internal/auth"
	"go-mqtt/internal/response"

	"github.com/gin-gonic/gin"
)

func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if authHeader == "" {
			response.Fail(c, http.StatusUnauthorized, "missing Authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Fail(c, http.StatusUnauthorized, "invalid Authorization format")
			c.Abort()
			return
		}

		claims, err := auth.ParseToken(strings.TrimSpace(parts[1]))
		if err != nil {
			response.Fail(c, http.StatusUnauthorized, "invalid or expired token")
			c.Abort()
			return
		}

		c.Set("auth_username", claims.Username)
		c.Set("auth_role", claims.Role)
		c.Next()
	}
}

func RequireRoles(roles ...string) gin.HandlerFunc {
	allow := map[string]struct{}{}
	for _, r := range roles {
		allow[strings.ToLower(strings.TrimSpace(r))] = struct{}{}
	}

	return func(c *gin.Context) {
		roleVal, ok := c.Get("auth_role")
		if !ok {
			response.Fail(c, http.StatusForbidden, "role missing")
			c.Abort()
			return
		}

		role, _ := roleVal.(string)
		if _, exists := allow[strings.ToLower(strings.TrimSpace(role))]; !exists {
			response.Fail(c, http.StatusForbidden, "permission denied")
			c.Abort()
			return
		}
		c.Next()
	}
}
