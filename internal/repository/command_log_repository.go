package repository

import (
	"go-mqtt/internal/config"
	"go-mqtt/internal/model"
	"time"

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

func (r *CommandLogRepository) UpdateByTraceID(traceID, result, message string, status int) (bool, error) {
	tx := r.DB.Model(&model.CommandLog{}).
		Where("trace_id = ? AND status = 0", traceID).
		Updates(map[string]any{
			"result":  result,
			"message": message,
			"status":  status,
			"done_at": time.Now().Unix(),
		})
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
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

func (r *CommandLogRepository) ListPendingForRetry(now int64, limit int) ([]model.CommandLog, error) {
	if limit <= 0 {
		limit = 100
	}
	var logs []model.CommandLog
	err := r.DB.Where("status = 0 AND next_retry_at > 0 AND next_retry_at <= ?", now).
		Order("id ASC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

func (r *CommandLogRepository) UpdateRetryPlan(traceID string, retryCount int, timeoutAt int64, nextRetryAt int64, message string) error {
	return r.DB.Model(&model.CommandLog{}).
		Where("trace_id = ? AND status = 0", traceID).
		Updates(map[string]any{
			"retry_count":   retryCount,
			"timeout_at":    timeoutAt,
			"next_retry_at": nextRetryAt,
			"result":        "retrying",
			"message":       message,
		}).Error
}

func (r *CommandLogRepository) MarkTimeout(traceID, message string) error {
	return r.DB.Model(&model.CommandLog{}).
		Where("trace_id = ? AND status = 0", traceID).
		Updates(map[string]any{
			"status":        2,
			"result":        "timeout",
			"message":       message,
			"done_at":       time.Now().Unix(),
			"next_retry_at": 0,
		}).Error
}
