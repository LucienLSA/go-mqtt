package auth

import (
	"os"
	"strings"
)

type User struct {
	Username string
	Password string
	Role     string
}

// LoadUsersFromEnv reads AUTH_USERS in format:
// 获取环境变量AUTH_USERS，格式为：username:password:role,username:password:role
func LoadUsersFromEnv() []User {
	users := make([]User, 0, 4)
	raw := strings.TrimSpace(os.Getenv("AUTH_USERS"))
	if raw == "" {
		users = append(users,
			User{Username: "admin", Password: "admin123", Role: "admin"},
			User{Username: "operator", Password: "operator123", Role: "operator"},
			User{Username: "viewer", Password: "viewer123", Role: "viewer"},
		)
		return users
	}

	pairs := strings.Split(raw, ",")
	for _, p := range pairs {
		parts := strings.Split(strings.TrimSpace(p), ":")
		if len(parts) != 3 {
			continue
		}
		u := strings.TrimSpace(parts[0])
		pw := strings.TrimSpace(parts[1])
		role := strings.TrimSpace(parts[2])
		if u == "" || pw == "" || role == "" {
			continue
		}
		users = append(users, User{Username: u, Password: pw, Role: role})
	}

	if len(users) == 0 {
		users = append(users, User{Username: "admin", Password: "admin123", Role: "admin"})
	}
	return users
}
