package model

import (
	"time"

	"gorm.io/gorm"
)

// AuthUser 管理后台账号
// role: admin/operator/viewer
// status: 1-启用, 0-禁用
type AuthUser struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	Username     string         `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string         `gorm:"size:255;not null" json:"-"`
	Role         string         `gorm:"size:32;index;not null" json:"role"`
	Status       int            `gorm:"default:1;index" json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}
