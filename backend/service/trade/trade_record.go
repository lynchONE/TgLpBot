package trade

import (
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type TradeRecordService struct{}

func NewTradeRecordService() *TradeRecordService {
	return &TradeRecordService{}
}

func DustAsset(symbol string, address common.Address, amount *big.Int) models.TradeRecordDustAsset {
	if amount == nil {
		amount = big.NewInt(0)
	}
	return models.TradeRecordDustAsset{
		Symbol:  strings.ToUpper(strings.TrimSpace(symbol)),
		Address: address.Hex(),
		Amount:  amount.String(),
	}
}

func ParseOpenDustAssets(raw string) []models.TradeRecordDustAsset {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var assets []models.TradeRecordDustAsset
	if err := json.Unmarshal([]byte(raw), &assets); err != nil {
		return nil
	}
	return normalizeDustAssets(assets)
}

func EncodeOpenDustAssets(assets []models.TradeRecordDustAsset) string {
	assets = normalizeDustAssets(assets)
	if len(assets) == 0 {
		return ""
	}
	b, err := json.Marshal(assets)
	if err != nil {
		return ""
	}
	return string(b)
}

func MergeOpenDustAssets(existing []models.TradeRecordDustAsset, additions []models.TradeRecordDustAsset) []models.TradeRecordDustAsset {
	return normalizeDustAssets(append(existing, additions...))
}

func flattenDustAssets(groups ...[]models.TradeRecordDustAsset) []models.TradeRecordDustAsset {
	var out []models.TradeRecordDustAsset
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func normalizeDustAssets(assets []models.TradeRecordDustAsset) []models.TradeRecordDustAsset {
	type bucket struct {
		asset  models.TradeRecordDustAsset
		amount *big.Int
	}
	buckets := make(map[string]*bucket)
	for _, asset := range assets {
		symbol := strings.ToUpper(strings.TrimSpace(asset.Symbol))
		addr := strings.TrimSpace(asset.Address)
		if common.IsHexAddress(addr) {
			addr = common.HexToAddress(addr).Hex()
		}
		amount, err := parseBigInt(asset.Amount)
		if err != nil || amount == nil || amount.Sign() <= 0 {
			continue
		}
		key := strings.ToLower(addr)
		if key == "" || !common.IsHexAddress(addr) {
			key = "symbol:" + symbol
		}
		if key == "symbol:" {
			continue
		}
		if cur, ok := buckets[key]; ok {
			cur.amount.Add(cur.amount, amount)
			if cur.asset.Symbol == "" && symbol != "" {
				cur.asset.Symbol = symbol
			}
			if cur.asset.Address == "" && addr != "" {
				cur.asset.Address = addr
			}
			continue
		}
		buckets[key] = &bucket{
			asset: models.TradeRecordDustAsset{
				Symbol:  symbol,
				Address: addr,
			},
			amount: new(big.Int).Set(amount),
		}
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]models.TradeRecordDustAsset, 0, len(keys))
	for _, key := range keys {
		item := buckets[key]
		if item == nil || item.amount == nil || item.amount.Sign() <= 0 {
			continue
		}
		item.asset.Amount = item.amount.String()
		out = append(out, item.asset)
	}
	return out
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
	openStableBeforeWei *big.Int,
	dust0Wei *big.Int,
	dust1Wei *big.Int,
	extraDust ...[]models.TradeRecordDustAsset,
) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	chain := config.NormalizeChain(task.Chain)
	now := time.Now()

	openSpent := openUSDTSpentWei
	if (openSpent == nil || openSpent.Sign() <= 0) && task.AmountUSDT > 0 {
		if fallback, err := convert.FloatUSDTToWei(task.AmountUSDT); err == nil && fallback.Sign() > 0 {
			openSpent = fallback
		}
	}
	openStableBefore := nonNilBigInt(openStableBeforeWei)
	openStableAfter := balanceAfterSpend(openStableBefore, openSpent)

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
		Chain:             chain,
		PoolVersion:       strings.TrimSpace(task.PoolVersion),
		PoolId:            strings.TrimSpace(task.PoolId),
		Exchange:          strings.TrimSpace(task.Exchange),
		Token0Symbol:      strings.TrimSpace(task.Token0Symbol),
		Token1Symbol:      strings.TrimSpace(task.Token1Symbol),
		OpenedAt:          now,
		OpenTxHash:        strings.TrimSpace(openTxHash),
		OpenUSDTSpent:     safeBigIntString(openSpent),
		OpenStableBefore:  safeBigIntString(openStableBefore),
		OpenStableAfter:   safeBigIntString(openStableAfter),
		OpenGasSpentWei:   safeBigIntString(openGasSpentWei),
		OpenDust0:         safeBigIntString(dust0Wei),
		OpenDust1:         safeBigIntString(dust1Wei),
		OpenExtraDust:     EncodeOpenDustAssets(flattenDustAssets(extraDust...)),
		Status:            models.TradeStatusOpen,
		CloseUSDTReceived: "0",
		CloseStableBefore: "0",
		CloseStableAfter:  "0",
		CloseGasSpentWei:  "0",
		ProfitUSDT:        "0",
		ProfitPct:         0,
	}

	return database.DB.Create(rec).Error
}

// AddToOpenUSDTSpent adds deltaWei (1e18) to the latest open trade record's OpenUSDTSpent.
// This is used when supplementing liquidity to an existing position so that PnL calculations
// accurately reflect the total invested amount.
func (s *TradeRecordService) AddToOpenUSDTSpent(task *models.StrategyTask, deltaWei *big.Int) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	if deltaWei == nil || deltaWei.Sign() <= 0 {
		return nil
	}

	var rec models.TradeRecord
	err := database.DB.
		Where("user_id = ? AND task_id = ? AND status = ?", task.UserID, task.ID, models.TradeStatusOpen).
		Order("opened_at DESC").
		First(&rec).Error
	if err != nil {
		return nil
	}

	existing, _ := parseBigInt(rec.OpenUSDTSpent)
	if existing == nil {
		existing = big.NewInt(0)
	}
	newSpent := new(big.Int).Add(existing, deltaWei)
	openStableBefore, _ := parseBigInt(rec.OpenStableBefore)
	openStableAfter := balanceAfterSpend(openStableBefore, newSpent)

	return database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).
		Updates(map[string]interface{}{
			"open_usdt_spent":   safeBigIntString(newSpent),
			"open_stable_after": safeBigIntString(openStableAfter),
		}).Error
}

func (s *TradeRecordService) ApplyAddLiquidityDelta(
	task *models.StrategyTask,
	stableSpentWei *big.Int,
	gasSpentWei *big.Int,
	dust0Wei *big.Int,
	dust1Wei *big.Int,
	extraDust ...models.TradeRecordDustAsset,
) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	var rec models.TradeRecord
	err := database.DB.
		Where("user_id = ? AND task_id = ? AND status = ?", task.UserID, task.ID, models.TradeStatusOpen).
		Order("opened_at DESC").
		First(&rec).Error
	if err != nil {
		return nil
	}

	openSpent, _ := parseBigInt(rec.OpenUSDTSpent)
	if openSpent == nil {
		openSpent = big.NewInt(0)
	}
	openGas, _ := parseBigInt(rec.OpenGasSpentWei)
	if openGas == nil {
		openGas = big.NewInt(0)
	}
	openDust0, _ := parseBigInt(rec.OpenDust0)
	if openDust0 == nil {
		openDust0 = big.NewInt(0)
	}
	openDust1, _ := parseBigInt(rec.OpenDust1)
	if openDust1 == nil {
		openDust1 = big.NewInt(0)
	}
	openStableBefore, _ := parseBigInt(rec.OpenStableBefore)

	nextSpent := new(big.Int).Add(openSpent, stableAmountTo18(task.Chain, stableSpentWei))
	nextGas := new(big.Int).Add(openGas, nonNilBigInt(gasSpentWei))
	nextDust0 := new(big.Int).Set(openDust0)
	nextDust1 := new(big.Int).Set(openDust1)
	nextStableAfter := balanceAfterSpend(openStableBefore, nextSpent)
	nextExtraDust := ParseOpenDustAssets(rec.OpenExtraDust)
	nextExtraDust = MergeOpenDustAssets(nextExtraDust, extraDust)

	primaryStable := common.HexToAddress("")
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(task.Chain); ok && common.IsHexAddress(cc.StableAddress) {
			primaryStable = common.HexToAddress(cc.StableAddress)
		}
	}
	if common.IsHexAddress(task.Token0Address) && common.HexToAddress(task.Token0Address) == primaryStable {
		if dust0Wei != nil && dust0Wei.Sign() > 0 {
			nextExtraDust = MergeOpenDustAssets(nextExtraDust, []models.TradeRecordDustAsset{
				DustAsset(task.Token0Symbol, common.HexToAddress(task.Token0Address), dust0Wei),
			})
		}
	} else {
		nextDust0.Add(nextDust0, nonNilBigInt(dust0Wei))
	}
	if common.IsHexAddress(task.Token1Address) && common.HexToAddress(task.Token1Address) == primaryStable {
		if dust1Wei != nil && dust1Wei.Sign() > 0 {
			nextExtraDust = MergeOpenDustAssets(nextExtraDust, []models.TradeRecordDustAsset{
				DustAsset(task.Token1Symbol, common.HexToAddress(task.Token1Address), dust1Wei),
			})
		}
	} else {
		nextDust1.Add(nextDust1, nonNilBigInt(dust1Wei))
	}

	return database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{
		"open_usdt_spent":    safeBigIntString(nextSpent),
		"open_stable_after":  safeBigIntString(nextStableAfter),
		"open_gas_spent_wei": safeBigIntString(nextGas),
		"open_dust0":         safeBigIntString(nextDust0),
		"open_dust1":         safeBigIntString(nextDust1),
		"open_extra_dust":    EncodeOpenDustAssets(nextExtraDust),
	}).Error
}

func (s *TradeRecordService) CloseLatestOpenRecord(
	task *models.StrategyTask,
	closeTxHash string,
	closeUSDTReceivedWei *big.Int,
	closeGasSpentWei *big.Int,
	closeStableBeforeWei *big.Int,
	nativePriceUSD float64,
) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	chain := config.NormalizeChain(task.Chain)
	var rec models.TradeRecord
	err := database.DB.
		Where("user_id = ? AND task_id = ? AND status = ?", task.UserID, task.ID, models.TradeStatusOpen).
		Order("opened_at DESC").
		First(&rec).Error

	now := time.Now()

	if err != nil {
		openSpent := big.NewInt(0)
		if task.AmountUSDT > 0 {
			if fallback, ferr := convert.FloatUSDTToWei(task.AmountUSDT); ferr == nil && fallback.Sign() > 0 {
				openSpent = fallback
			}
		}
		totalGasWei := nonNilBigInt(closeGasSpentWei)
		totalGasUSDT := calcGasUSDT(totalGasWei, nativePriceUSD)
		profit := tradeProfitUSDT(closeUSDTReceivedWei, openSpent, totalGasUSDT)
		profitPct := calcProfitPct(profit, openSpent)
		closeStableBefore := nonNilBigInt(closeStableBeforeWei)
		closeStableAfter := balanceAfterReceive(closeStableBefore, closeUSDTReceivedWei)

		closed := &models.TradeRecord{
			UserID:            task.UserID,
			TaskID:            task.ID,
			Chain:             chain,
			PoolVersion:       strings.TrimSpace(task.PoolVersion),
			PoolId:            strings.TrimSpace(task.PoolId),
			Exchange:          strings.TrimSpace(task.Exchange),
			Token0Symbol:      strings.TrimSpace(task.Token0Symbol),
			Token1Symbol:      strings.TrimSpace(task.Token1Symbol),
			OpenedAt:          now,
			OpenTxHash:        "",
			OpenUSDTSpent:     safeBigIntString(openSpent),
			OpenStableBefore:  "0",
			OpenStableAfter:   "0",
			OpenGasSpentWei:   "0",
			ClosedAt:          &now,
			CloseTxHash:       strings.TrimSpace(closeTxHash),
			CloseUSDTReceived: safeBigIntString(closeUSDTReceivedWei),
			CloseStableBefore: safeBigIntString(closeStableBefore),
			CloseStableAfter:  safeBigIntString(closeStableAfter),
			CloseGasSpentWei:  safeBigIntString(closeGasSpentWei),
			TotalGasUSDT:      safeBigIntString(totalGasUSDT),
			ProfitUSDT:        profit.String(),
			ProfitPct:         profitPct,
			Status:            models.TradeStatusClosed,
		}
		return database.DB.Create(closed).Error
	}

	openSpent, _ := parseBigInt(rec.OpenUSDTSpent)
	if openSpent == nil {
		openSpent = big.NewInt(0)
	}
	openGasWei, _ := parseBigInt(rec.OpenGasSpentWei)
	if openGasWei == nil {
		openGasWei = big.NewInt(0)
	}
	openStableBefore, _ := parseBigInt(rec.OpenStableBefore)

	fallbackOpen := false
	if openSpent.Sign() <= 0 && task.AmountUSDT > 0 {
		if fallback, err := convert.FloatUSDTToWei(task.AmountUSDT); err == nil && fallback.Sign() > 0 {
			openSpent = fallback
			fallbackOpen = true
		}
	}

	totalGasWei := new(big.Int).Add(openGasWei, nonNilBigInt(closeGasSpentWei))
	totalGasUSDT := calcGasUSDT(totalGasWei, nativePriceUSD)
	profit := tradeProfitUSDT(closeUSDTReceivedWei, openSpent, totalGasUSDT)
	profitPct := calcProfitPct(profit, openSpent)
	closeStableBefore := nonNilBigInt(closeStableBeforeWei)
	closeStableAfter := balanceAfterReceive(closeStableBefore, closeUSDTReceivedWei)

	updates := map[string]interface{}{
		"closed_at":           &now,
		"chain":               chain,
		"close_tx_hash":       strings.TrimSpace(closeTxHash),
		"close_usdt_received": safeBigIntString(closeUSDTReceivedWei),
		"close_stable_before": safeBigIntString(closeStableBefore),
		"close_stable_after":  safeBigIntString(closeStableAfter),
		"close_gas_spent_wei": safeBigIntString(closeGasSpentWei),
		"total_gas_usdt":      safeBigIntString(totalGasUSDT),
		"profit_usdt":         profit.String(),
		"profit_pct":          profitPct,
		"status":              models.TradeStatusClosed,
	}
	if fallbackOpen {
		updates["open_usdt_spent"] = openSpent.String()
		updates["open_stable_after"] = safeBigIntString(balanceAfterSpend(openStableBefore, openSpent))
	}

	return database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(updates).Error
}

// ApplyExitDelta accumulates exit-phase deltas (received USDT + gas spent) into the latest trade record.
// It prefers the latest OPEN record; if none exists, it falls back to the latest record for this task
// (so a previously-closed-too-early record can still be corrected across retries).
//
// When finalize=true, it will also mark the record as CLOSED and recompute Profit/TotalGasUSDT.
func (s *TradeRecordService) ApplyExitDelta(
	task *models.StrategyTask,
	closeTxHash string,
	closeUSDTReceivedDeltaWei *big.Int,
	closeGasSpentDeltaWei *big.Int,
	closeStableBeforeWei *big.Int,
	finalize bool,
	nativePriceUSD float64,
) (*models.TradeRecord, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	chain := config.NormalizeChain(task.Chain)
	var rec models.TradeRecord
	err := database.DB.
		Where("user_id = ? AND task_id = ? AND status = ?", task.UserID, task.ID, models.TradeStatusOpen).
		Order("opened_at DESC").
		First(&rec).Error
	if err != nil {
		err2 := database.DB.
			Where("user_id = ? AND task_id = ?", task.UserID, task.ID).
			Order("opened_at DESC").
			First(&rec).Error
		if err2 != nil {
			return nil, err2
		}
	}

	curReceived, _ := parseBigInt(rec.CloseUSDTReceived)
	if curReceived == nil {
		curReceived = big.NewInt(0)
	}
	curGas, _ := parseBigInt(rec.CloseGasSpentWei)
	if curGas == nil {
		curGas = big.NewInt(0)
	}

	deltaReceived := nonNilBigInt(closeUSDTReceivedDeltaWei)
	if deltaReceived.Sign() < 0 {
		deltaReceived = big.NewInt(0)
	}
	deltaGas := nonNilBigInt(closeGasSpentDeltaWei)
	if deltaGas.Sign() < 0 {
		deltaGas = big.NewInt(0)
	}

	newReceived := new(big.Int).Add(curReceived, deltaReceived)
	if newReceived.Sign() < 0 {
		newReceived = big.NewInt(0)
	}
	newGas := new(big.Int).Add(curGas, deltaGas)
	if newGas.Sign() < 0 {
		newGas = big.NewInt(0)
	}

	closeTxHash = strings.TrimSpace(closeTxHash)

	updates := map[string]interface{}{
		"close_usdt_received": newReceived.String(),
		"close_gas_spent_wei": newGas.String(),
	}
	if chain != "" && strings.TrimSpace(rec.Chain) != chain {
		updates["chain"] = chain
		rec.Chain = chain
	}
	if strings.TrimSpace(rec.CloseTxHash) == "" && closeTxHash != "" {
		updates["close_tx_hash"] = closeTxHash
		rec.CloseTxHash = closeTxHash
	}

	openSpent, _ := parseBigInt(rec.OpenUSDTSpent)
	if openSpent == nil {
		openSpent = big.NewInt(0)
	}
	openStableBefore, _ := parseBigInt(rec.OpenStableBefore)

	fallbackOpen := false
	if openSpent.Sign() <= 0 && task.AmountUSDT > 0 {
		if fallback, err := convert.FloatUSDTToWei(task.AmountUSDT); err == nil && fallback.Sign() > 0 {
			openSpent = fallback
			fallbackOpen = true
		}
	}
	if fallbackOpen {
		updates["open_usdt_spent"] = openSpent.String()
		updates["open_stable_after"] = safeBigIntString(balanceAfterSpend(openStableBefore, openSpent))
		rec.OpenUSDTSpent = openSpent.String()
		rec.OpenStableAfter = safeBigIntString(balanceAfterSpend(openStableBefore, openSpent))
	}

	existingCloseStableBefore, _ := parseBigInt(rec.CloseStableBefore)
	if existingCloseStableBefore == nil {
		existingCloseStableBefore = big.NewInt(0)
	}
	if existingCloseStableBefore.Sign() <= 0 && closeStableBeforeWei != nil && closeStableBeforeWei.Sign() > 0 {
		existingCloseStableBefore = new(big.Int).Set(closeStableBeforeWei)
	}
	closeStableAfter := balanceAfterReceive(existingCloseStableBefore, newReceived)
	updates["close_stable_before"] = safeBigIntString(existingCloseStableBefore)
	updates["close_stable_after"] = safeBigIntString(closeStableAfter)

	if finalize {
		openGasWei, _ := parseBigInt(rec.OpenGasSpentWei)
		if openGasWei == nil {
			openGasWei = big.NewInt(0)
		}

		totalGasWei := new(big.Int).Add(openGasWei, newGas)
		totalGasUSDT := calcGasUSDT(totalGasWei, nativePriceUSD)
		profit := tradeProfitUSDT(newReceived, openSpent, totalGasUSDT)
		profitPct := calcProfitPct(profit, openSpent)

		now := time.Now()
		updates["closed_at"] = &now
		updates["total_gas_usdt"] = safeBigIntString(totalGasUSDT)
		updates["profit_usdt"] = profit.String()
		updates["profit_pct"] = profitPct
		updates["status"] = models.TradeStatusClosed

		rec.ClosedAt = &now
		rec.TotalGasUSDT = safeBigIntString(totalGasUSDT)
		rec.ProfitUSDT = profit.String()
		rec.ProfitPct = profitPct
		rec.Status = models.TradeStatusClosed
	}

	if err := database.DB.Model(&models.TradeRecord{}).Where("id = ?", rec.ID).Updates(updates).Error; err != nil {
		return nil, err
	}

	rec.CloseUSDTReceived = newReceived.String()
	rec.CloseStableBefore = safeBigIntString(existingCloseStableBefore)
	rec.CloseStableAfter = safeBigIntString(closeStableAfter)
	rec.CloseGasSpentWei = newGas.String()

	return &rec, nil
}

func calcGasUSDT(gasWei *big.Int, nativePriceUSD float64) *big.Int {
	if gasWei == nil || gasWei.Sign() <= 0 || nativePriceUSD <= 0 {
		return big.NewInt(0)
	}
	gasFloat := new(big.Float).SetInt(gasWei)
	priceFloat := new(big.Float).SetFloat64(nativePriceUSD)
	result := new(big.Float).Mul(gasFloat, priceFloat)
	resultInt, _ := result.Int(nil)
	return resultInt
}

func tradeProfitUSDT(closeReceived, openSpent, totalGasUSDT *big.Int) *big.Int {
	profit := new(big.Int).Sub(nonNilBigInt(closeReceived), nonNilBigInt(openSpent))
	profit.Sub(profit, nonNilBigInt(totalGasUSDT))
	return profit
}

func RealizedProfitUSDTFromBalanceSnapshots(record *models.TradeRecord) (*big.Int, bool, error) {
	if record == nil {
		return nil, false, fmt.Errorf("trade record is nil")
	}
	if strings.TrimSpace(record.OpenStableBefore) == "" || strings.TrimSpace(record.CloseStableAfter) == "" {
		return nil, false, nil
	}
	openStableBefore, err := parseBigInt(record.OpenStableBefore)
	if err != nil {
		return nil, false, fmt.Errorf("parse open stable before: %w", err)
	}
	closeStableAfter, err := parseBigInt(record.CloseStableAfter)
	if err != nil {
		return nil, false, fmt.Errorf("parse close stable after: %w", err)
	}
	if openStableBefore == nil || openStableBefore.Sign() <= 0 {
		return nil, false, nil
	}
	totalGasUSDT, err := parseBigInt(record.TotalGasUSDT)
	if err != nil {
		return nil, false, fmt.Errorf("parse total gas usdt: %w", err)
	}
	profit := new(big.Int).Sub(nonNilBigInt(closeStableAfter), openStableBefore)
	profit.Sub(profit, nonNilBigInt(totalGasUSDT))
	return profit, true, nil
}

func balanceAfterSpend(before, spent *big.Int) *big.Int {
	after := new(big.Int).Sub(nonNilBigInt(before), nonNilBigInt(spent))
	if after.Sign() < 0 {
		return big.NewInt(0)
	}
	return after
}

func balanceAfterReceive(before, received *big.Int) *big.Int {
	return new(big.Int).Add(nonNilBigInt(before), nonNilBigInt(received))
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

func stableAmountTo18(chain string, amount *big.Int) *big.Int {
	if amount == nil || amount.Sign() <= 0 {
		return big.NewInt(0)
	}
	decimals := 18
	if config.AppConfig != nil {
		if cc, ok := config.AppConfig.GetChainConfig(chain); ok && cc.StableDecimals > 0 {
			decimals = cc.StableDecimals
		}
	}
	scaled, err := convert.ScaleDecimals(amount, decimals, 18)
	if err != nil || scaled == nil {
		return new(big.Int).Set(amount)
	}
	return scaled
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
