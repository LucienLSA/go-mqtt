package repository

import (
	"go-mqtt/internal/config"
	"go-mqtt/internal/model"

	"gorm.io/gorm"
)

// SensorDataRepository 负责传感器数据写入
type SensorDataRepository struct {
	DB *gorm.DB
}

func NewSensorDataRepository() *SensorDataRepository {
	return &SensorDataRepository{DB: config.DB}
}

func (r *SensorDataRepository) Create(data *model.SensorData) error {
	return r.DB.Create(data).Error
}
