package repository

import (
	"go-mqtt/internal/config"
	"go-mqtt/internal/model"

	"gorm.io/gorm"
)

// 定义数据库对象
type DeviceRepository struct {
	DB *gorm.DB
}

func NewDeviceRepository() *DeviceRepository {
	return &DeviceRepository{DB: config.DB}
}

// 创建设备
func (r *DeviceRepository) Create(device *model.Device) error {
	return r.DB.Create(device).Error
}

// 根据ID获取设备
func (r *DeviceRepository) GetByID(id uint) (*model.Device, error) {
	var device model.Device
	err := r.DB.First(&device, id).Error
	return &device, err
}

// 根据DeviceID获取设备
func (r *DeviceRepository) GetByDeviceID(deviceID string) (*model.Device, error) {
	var device model.Device
	err := r.DB.Where("device_id = ?", deviceID).First(&device).Error
	return &device, err
}

// 根据DeviceID更新在线状态
func (r *DeviceRepository) UpdateStatusByDeviceID(deviceID string, status int) error {
	return r.DB.Model(&model.Device{}).Where("device_id = ?", deviceID).Update("status", status).Error
}

// 获取所有设备
func (r *DeviceRepository) GetAll() ([]model.Device, error) {
	var devices []model.Device
	err := r.DB.Find(&devices).Error
	return devices, err
}

// 更新设备
func (r *DeviceRepository) Update(device *model.Device) error {
	return r.DB.Save(device).Error
}

// 删除设备
func (r *DeviceRepository) Delete(id uint) error {
	return r.DB.Delete(&model.Device{}, id).Error
}
