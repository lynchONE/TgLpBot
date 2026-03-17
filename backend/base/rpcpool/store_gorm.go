package rpcpool

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type GormStore struct{}

func NewGormStore() *GormStore { return &GormStore{} }

func (s *GormStore) db() (*gorm.DB, error) {
	if database.DB == nil {
		return nil, errors.New("database not initialized")
	}
	return database.DB, nil
}

func (s *GormStore) ListAll(ctx context.Context) ([]models.RpcEndpoint, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	var out []models.RpcEndpoint
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	if err := q.Order("chain asc, transport asc, is_current desc, id asc").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) List(ctx context.Context, chain string, transport string) ([]models.RpcEndpoint, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	var out []models.RpcEndpoint
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	if err := q.Where("chain = ? AND transport = ?", chain, transport).
		Order("is_current desc, id asc").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) GetByID(ctx context.Context, id uint) (*models.RpcEndpoint, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}
	var ep models.RpcEndpoint
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	if err := q.First(&ep, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ep, nil
}

func (s *GormStore) Create(ctx context.Context, ep *models.RpcEndpoint) error {
	if ep == nil {
		return fmt.Errorf("rpc endpoint is nil")
	}
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	return q.Create(ep).Error
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
	return q.Model(&models.RpcEndpoint{}).Where("id = ?", id).Updates(updates).Error
}

func (s *GormStore) SetCurrent(ctx context.Context, chain string, transport string, id uint) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	tx := db
	if ctx != nil {
		tx = tx.WithContext(ctx)
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		var ep models.RpcEndpoint
		if err := tx.First(&ep, id).Error; err != nil {
			return err
		}
		if ep.Chain != chain || ep.Transport != transport {
			return fmt.Errorf("rpc endpoint mismatch: id=%d chain=%s transport=%s", id, chain, transport)
		}

		if err := tx.Model(&models.RpcEndpoint{}).
			Where("chain = ? AND transport = ?", chain, transport).
			Update("is_current", false).Error; err != nil {
			return err
		}
		return tx.Model(&models.RpcEndpoint{}).
			Where("id = ?", id).
			Update("is_current", true).Error
	})
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
	result := q.Delete(&models.RpcEndpoint{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("endpoint not found")
	}
	return nil
}

func (s *GormStore) UnsetCurrent(ctx context.Context, chain string, transport string) error {
	db, err := s.db()
	if err != nil {
		return err
	}
	q := db
	if ctx != nil {
		q = q.WithContext(ctx)
	}
	return q.Model(&models.RpcEndpoint{}).
		Where("chain = ? AND transport = ?", chain, transport).
		Update("is_current", false).Error
}
