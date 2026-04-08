package model

import (
	"time"

	"gorm.io/gorm"
)

// 设备信息表
type Device struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	DeviceID     string         `gorm:"size:64;uniqueIndex;not null" json:"device_id"`
	DeviceSecret string         `gorm:"size:64;not null" json:"-"`
	Name         string         `gorm:"size:128" json:"name"`
	GroupID      uint           `gorm:"default:0" json:"group_id"`
	Status       int            `gorm:"default:0;comment:0-离线,1-在线" json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// 传感器数据
type SensorData struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	DeviceID  string         `gorm:"size:64;index;not null" json:"device_id"`
	Temp      float64        `gorm:"type:decimal(5,2)" json:"temp"`
	Humi      float64        `gorm:"type:decimal(5,2)" json:"humi"`
	Voltage   float64        `gorm:"type:decimal(5,2)" json:"voltage"`
	Status    int            `gorm:"default:1" json:"status"`
	Ts        int64          `gorm:"index" json:"ts"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// 运行日志
type CommandLog struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	DeviceID    string         `gorm:"size:64;index;not null" json:"device_id"`
	TraceID     string         `gorm:"size:64;index;not null" json:"trace_id"`
	Topic       string         `gorm:"size:128" json:"topic"`
	Payload     string         `gorm:"type:text" json:"payload"`
	Command     string         `gorm:"size:256" json:"command"`
	Result      string         `gorm:"size:256" json:"result"`
	Status      int            `gorm:"default:1" json:"status"`
	RetryCount  int            `gorm:"default:0" json:"retry_count"`
	MaxRetry    int            `gorm:"default:2" json:"max_retry"`
	TimeoutAt   int64          `gorm:"index" json:"timeout_at"`
	NextRetryAt int64          `gorm:"index" json:"next_retry_at"`
	DoneAt      int64          `gorm:"default:0" json:"done_at"`
	Message     string         `gorm:"size:512" json:"message"`
	CreatedAt   time.Time      `json:"created_at"`
	DealetedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}
