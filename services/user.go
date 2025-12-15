package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// UserService handles user operations
type UserService struct{}

// NewUserService creates a new user service
func NewUserService() *UserService {
	return &UserService{}
}

// GetOrCreateUser gets or creates a user by Telegram ID
func (s *UserService) GetOrCreateUser(telegramID int64, username, firstName, lastName, languageCode string) (*models.User, error) {
	var user models.User

	err := database.DB.Where("telegram_id = ?", telegramID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create new user
			user = models.User{
				TelegramID:   telegramID,
				Username:     username,
				FirstName:    firstName,
				LastName:     lastName,
				LanguageCode: languageCode,
				IsActive:     true,
			}

			if err := database.DB.Create(&user).Error; err != nil {
				return nil, fmt.Errorf("failed to create user: %w", err)
			}

			return &user, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Update user info if changed
	updated := false
	if user.Username != username {
		user.Username = username
		updated = true
	}
	if user.FirstName != firstName {
		user.FirstName = firstName
		updated = true
	}
	if user.LastName != lastName {
		user.LastName = lastName
		updated = true
	}
	if user.LanguageCode != languageCode {
		user.LanguageCode = languageCode
		updated = true
	}

	if updated {
		if err := database.DB.Save(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
	}

	return &user, nil
}

// GetUserByTelegramID gets a user by Telegram ID
func (s *UserService) GetUserByTelegramID(telegramID int64) (*models.User, error) {
	var user models.User
	err := database.DB.Where("telegram_id = ?", telegramID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// GetUserByID gets a user by ID
func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	err := database.DB.First(&user, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// UpdateUser updates a user
func (s *UserService) UpdateUser(user *models.User) error {
	return database.DB.Save(user).Error
}

// DeactivateUser deactivates a user
func (s *UserService) DeactivateUser(id uint) error {
	return database.DB.Model(&models.User{}).Where("id = ?", id).Update("is_active", false).Error
}

// ActivateUser activates a user
func (s *UserService) ActivateUser(id uint) error {
	return database.DB.Model(&models.User{}).Where("id = ?", id).Update("is_active", true).Error
}
