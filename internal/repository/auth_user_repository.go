package repository

import (
	"go-mqtt/internal/config"
	"go-mqtt/internal/model"

	"gorm.io/gorm"
)

type AuthUserRepository struct {
	DB *gorm.DB
}

func NewAuthUserRepository() *AuthUserRepository {
	return &AuthUserRepository{DB: config.DB}
}

func (r *AuthUserRepository) GetByUsername(username string) (*model.AuthUser, error) {
	var user model.AuthUser
	err := r.DB.Where("username = ?", username).First(&user).Error
	return &user, err
}

func (r *AuthUserRepository) Create(user *model.AuthUser) error {
	return r.DB.Create(user).Error
}
