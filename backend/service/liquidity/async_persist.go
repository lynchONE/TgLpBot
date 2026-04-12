package liquidity

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"log"

	"gorm.io/gorm/clause"
)

func (s *LiquidityService) runAsync(label string, fn func() error) {
	if fn == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Liquidity] async task panic: %s: %v", label, r)
			}
		}()
		if err := fn(); err != nil {
			log.Printf("[Liquidity] async task failed: %s: %v", label, err)
		}
	}()
}

func (s *LiquidityService) upsertTransactionRecord(record models.Transaction) error {
	if database.DB == nil {
		return fmt.Errorf("database not initialized")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}
	return database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tx_hash"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":           record.UserID,
			"chain":             record.Chain,
			"task_id":           record.TaskID,
			"type":              record.Type,
			"status":            record.Status,
			"provider":          record.Provider,
			"from_address":      record.FromAddress,
			"to_address":        record.ToAddress,
			"token_in_address":  record.TokenInAddress,
			"token_out_address": record.TokenOutAddress,
			"amount_in":         record.AmountIn,
			"amount_out":        record.AmountOut,
			"gas_price":         record.GasPrice,
			"gas_used":          record.GasUsed,
			"block_number":      record.BlockNumber,
			"error_message":     record.ErrorMessage,
			"updated_at":        clause.Expr{SQL: "CURRENT_TIMESTAMP"},
		}),
	}).Create(&record).Error
}

func (s *LiquidityService) persistTransactionRecordAsync(label string, record models.Transaction) {
	rec := record
	s.runAsync(label, func() error {
		return s.upsertTransactionRecord(rec)
	})
}
