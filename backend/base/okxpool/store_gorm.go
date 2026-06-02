package okxpool

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type Store interface {
	ListAll(ctx context.Context) ([]models.OKXAPIConfig, error)
	ListEnabled(ctx context.Context) ([]models.OKXAPIConfig, error)
	GetByID(ctx context.Context, id uint) (*models.OKXAPIConfig, error)
	Create(ctx context.Context, row *models.OKXAPIConfig) error
	UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error
	DeleteByID(ctx context.Context, id uint) error
	SetCurrent(ctx context.Context, id uint) error
	UnsetCurrent(ctx context.Context) error
}

type GormStore struct{}

func NewGormStore() *GormStore { return &GormStore{} }

func (s *GormStore) db() (*gorm.DB, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	return database.DB, nil
}

func (s *GormStore) ListAll(ctx context.Context) ([]models.OKXAPIConfig, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	var out []models.OKXAPIConfig
	if err := q.Order("is_current desc, is_enabled desc, id asc").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) ListEnabled(ctx context.Context) ([]models.OKXAPIConfig, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	var out []models.OKXAPIConfig
	if err := q.Where("is_enabled = ?", true).Order("is_current desc, id asc").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) GetByID(ctx context.Context, id uint) (*models.OKXAPIConfig, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	var row models.OKXAPIConfig
	if err := q.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (s *GormStore) Create(ctx context.Context, row *models.OKXAPIConfig) error {
	if row == nil {
		return fmt.Errorf("okx api config is nil")
	}
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	return q.Create(row).Error
}

func (s *GormStore) UpdateByID(ctx context.Context, id uint, updates map[string]interface{}) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	return q.Model(&models.OKXAPIConfig{}).Where("id = ?", id).Updates(updates).Error
}

func (s *GormStore) DeleteByID(ctx context.Context, id uint) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	result := q.Delete(&models.OKXAPIConfig{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("okx api config not found")
	}
	return nil
}

func (s *GormStore) SetCurrent(ctx context.Context, id uint) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	tx := db
	if ctx != nil {
		tx = tx.WithContext(ctx)
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		var row models.OKXAPIConfig
		if err := tx.First(&row, id).Error; err != nil {
			return err
		}
		if !row.IsEnabled {
			return fmt.Errorf("okx api config is disabled")
		}
		if err := tx.Model(&models.OKXAPIConfig{}).Where("1 = 1").Update("is_current", false).Error; err != nil {
			return err
		}
		return tx.Model(&models.OKXAPIConfig{}).Where("id = ?", id).Update("is_current", true).Error
	})
}

func (s *GormStore) UnsetCurrent(ctx context.Context) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	return q.Model(&models.OKXAPIConfig{}).Where("1 = 1").Update("is_current", false).Error
}
