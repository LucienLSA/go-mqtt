package repository

import (
	"go-mqtt/internal/config"
	"go-mqtt/internal/model"

	"gorm.io/gorm"
)

// CommandLogRepository 负责控制命令日志
// status: 0-pending, 1-success, 2-failed
type CommandLogRepository struct {
	DB *gorm.DB
}

func NewCommandLogRepository() *CommandLogRepository {
	return &CommandLogRepository{DB: config.DB}
}

func (r *CommandLogRepository) Create(log *model.CommandLog) error {
	return r.DB.Create(log).Error
}

func (r *CommandLogRepository) UpdateByTraceID(traceID, result, message string, status int) error {
	return r.DB.Model(&model.CommandLog{}).
		Where("trace_id = ?", traceID).
		Updates(map[string]any{
			"result":  result,
			"message": message,
			"status":  status,
		}).Error
}

func (r *CommandLogRepository) ListByDeviceID(deviceID string, limit int) ([]model.CommandLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var logs []model.CommandLog
	err := r.DB.Where("device_id = ?", deviceID).
		Order("id DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}
