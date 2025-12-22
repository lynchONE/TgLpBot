package services

import (
	"TgLpBot/database"
	"TgLpBot/models"
	"fmt"
	"math/big"
	"strings"
	"time"
)

type TradeRecordService struct{}

func NewTradeRecordService() *TradeRecordService {
	return &TradeRecordService{}
}

func (s *TradeRecordService) GetLatestOpenRecord(userID uint, taskID uint) (*models.TradeRecord, error) {
	var rec models.TradeRecord
	err := database.DB.
		Where("user_id = ? AND task_id = ? AND status = ?", userID, taskID, models.TradeStatusOpen).
		Order("opened_at DESC").
		First(&rec).Error
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *TradeRecordService) CreateOpenRecord(
	task *models.StrategyTask,
	openTxHash string,
	openUSDTSpentWei *big.Int,
	openGasSpentWei *big.Int,
	dust0Wei *big.Int,
	dust1Wei *big.Int,
) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	now := time.Now()

	// If there is any dangling open record for this task, mark it as orphaned.
	_ = database.DB.Model(&models.TradeRecord{}).
		Where("user_id = ? AND task_id = ? AND status = ?", task.UserID, task.ID, models.TradeStatusOpen).
		Updates(map[string]interface{}{
			"status":    models.TradeStatusOrphaned,
			"closed_at": &now,
		}).Error

	rec := &models.TradeRecord{
		UserID:            task.UserID,
		TaskID:            task.ID,
		PoolVersion:       strings.TrimSpace(task.PoolVersion),
		PoolId:            strings.TrimSpace(task.PoolId),
		Exchange:          strings.TrimSpace(task.Exchange),
		Token0Symbol:      strings.TrimSpace(task.Token0Symbol),
		Token1Symbol:      strings.TrimSpace(task.Token1Symbol),
		OpenedAt:          now,
		OpenTxHash:        strings.TrimSpace(openTxHash),
		OpenUSDTSpent:     safeBigIntString(openUSDTSpentWei),
		OpenGasSpentWei:   safeBigIntString(openGasSpentWei),
		OpenDust0:         safeBigIntString(dust0Wei),
		OpenDust1:         safeBigIntString(dust1Wei),
		Status:            models.TradeStatusOpen,
		CloseUSDTReceived: "0",
		CloseGasSpentWei:  "0",
		ProfitUSDT:        "0",
		ProfitPct:         0,
	}

	return database.DB.Create(rec).Error
}

func (s *TradeRecordService) CloseLatestOpenRecord(task *models.StrategyTask, closeTxHash string, closeUSDTReceivedWei, closeGasSpentWei *big.Int) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	var rec models.TradeRecord
	err := database.DB.
		Where("user_id = ? AND task_id = ? AND status = ?", task.UserID, task.ID, models.TradeStatusOpen).
		Order("opened_at DESC").
		First(&rec).Error

	now := time.Now()

	// If no open record exists (e.g. legacy tasks created before this feature),
	// create a closed record with unknown open fields to avoid losing the exit summary.
	if err != nil {
		openZero := big.NewInt(0)
		profit := new(big.Int).Sub(nonNilBigInt(closeUSDTReceivedWei), openZero)
		profitPct := calcProfitPct(profit, openZero)

		closed := &models.TradeRecord{
			UserID:            task.UserID,
			TaskID:            task.ID,
			PoolVersion:       strings.TrimSpace(task.PoolVersion),
			PoolId:            strings.TrimSpace(task.PoolId),
			Exchange:          strings.TrimSpace(task.Exchange),
			Token0Symbol:      strings.TrimSpace(task.Token0Symbol),
			Token1Symbol:      strings.TrimSpace(task.Token1Symbol),
			OpenedAt:          now,
			OpenTxHash:        "",
			OpenUSDTSpent:     "0",
			OpenGasSpentWei:   "0",
			ClosedAt:          &now,
			CloseTxHash:       strings.TrimSpace(closeTxHash),
			CloseUSDTReceived: safeBigIntString(closeUSDTReceivedWei),
			CloseGasSpentWei:  safeBigIntString(closeGasSpentWei),
			ProfitUSDT:        profit.String(),
			ProfitPct:         profitPct,
			Status:            models.TradeStatusClosed,
		}
		return database.DB.Create(closed).Error
	}

	openSpent, _ := parseBigInt(rec.OpenUSDTSpent)
	profit := new(big.Int).Sub(nonNilBigInt(closeUSDTReceivedWei), openSpent)
	profitPct := calcProfitPct(profit, openSpent)

	updates := map[string]interface{}{
		"closed_at":           &now,
		"close_tx_hash":       strings.TrimSpace(closeTxHash),
		"close_usdt_received": safeBigIntString(closeUSDTReceivedWei),
		"close_gas_spent_wei": safeBigIntString(closeGasSpentWei),
		"profit_usdt":         profit.String(),
		"profit_pct":          profitPct,
		"status":              models.TradeStatusClosed,
	}

	return database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(updates).Error
}

func safeBigIntString(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

func nonNilBigInt(v *big.Int) *big.Int {
	if v == nil {
		return big.NewInt(0)
	}
	return v
}

func parseBigInt(s string) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return big.NewInt(0), nil
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, ok := new(big.Int).SetString(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"), 16)
		if !ok {
			return nil, fmt.Errorf("invalid hex integer")
		}
		return v, nil
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal integer")
	}
	return v, nil
}

func calcProfitPct(profit, openSpent *big.Int) float64 {
	if openSpent == nil || openSpent.Sign() <= 0 {
		return 0
	}
	pf := new(big.Float).SetInt(profit)
	of := new(big.Float).SetInt(openSpent)
	ratio := new(big.Float).Quo(pf, of)
	ratio.Mul(ratio, big.NewFloat(100))
	v, _ := ratio.Float64()
	return v
}
