package web_server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
)

type tradeHistoryRequest struct {
	InitData string `json:"initData"`
	Chain    string `json:"chain,omitempty"`
	Status   string `json:"status,omitempty"` // open, closed, aborted, orphaned or empty for all
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

type tradeRecordRow struct {
	ID                uint    `json:"id"`
	TaskID            uint    `json:"task_id"`
	Chain             string  `json:"chain"`
	PoolVersion       string  `json:"pool_version"`
	PoolID            string  `json:"pool_id"`
	Exchange          string  `json:"exchange"`
	Token0Symbol      string  `json:"token0_symbol"`
	Token1Symbol      string  `json:"token1_symbol"`
	OpenedAt          string  `json:"opened_at"`
	OpenTxHash        string  `json:"open_tx_hash"`
	OpenUSDTSpent     float64 `json:"open_usdt_spent"`
	OpenStableBefore  float64 `json:"open_stable_before,omitempty"`
	OpenStableAfter   float64 `json:"open_stable_after,omitempty"`
	OpenGasSpentWei   float64 `json:"open_gas_spent_wei"`
	ClosedAt          string  `json:"closed_at,omitempty"`
	CloseTxHash       string  `json:"close_tx_hash,omitempty"`
	CloseUSDTRecv     float64 `json:"close_usdt_received,omitempty"`
	CloseStableBefore float64 `json:"close_stable_before,omitempty"`
	CloseStableAfter  float64 `json:"close_stable_after,omitempty"`
	CloseGasSpentWei  float64 `json:"close_gas_spent_wei,omitempty"`
	TotalGasUSDT      float64 `json:"total_gas_usdt,omitempty"`
	ProfitUSDT        float64 `json:"profit_usdt,omitempty"`
	ProfitPct         float64 `json:"profit_pct"`
	Status            string  `json:"status"`
	OpenTxURL         string  `json:"open_tx_url,omitempty"`
	CloseTxURL        string  `json:"close_tx_url,omitempty"`
}

type tradeHistoryResponse struct {
	OK      bool             `json:"ok"`
	Total   int64            `json:"total"`
	Records []tradeRecordRow `json:"records"`
}

func explorerTxURLHelper(chain string, txHash string) string {
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return ""
	}
	if url := config.ExplorerTxURL(chain, txHash); url != "" {
		return url
	}
	switch config.NormalizeChain(chain) {
	case "base":
		return "https://basescan.org/tx/" + txHash
	default:
		return "https://bscscan.com/tx/" + txHash
	}
}

func (s *Server) handleTradeHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req tradeHistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	db := database.DB.Model(&models.TradeRecord{}).Where("user_id = ?", user.ID)

	chain := strings.TrimSpace(req.Chain)
	if chain != "" {
		db = db.Where("chain = ?", config.NormalizeChain(chain))
	}
	statusFilter := strings.TrimSpace(req.Status)
	if statusFilter != "" {
		db = db.Where("status = ?", statusFilter)
	}

	var total int64
	db.Count(&total)

	var records []models.TradeRecord
	if err := db.Order("opened_at DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		http.Error(w, "failed to query trade history", http.StatusInternalServerError)
		return
	}

	rows := make([]tradeRecordRow, 0, len(records))
	for _, rec := range records {
		row := tradeRecordRow{
			ID:               rec.ID,
			TaskID:           rec.TaskID,
			Chain:            rec.Chain,
			PoolVersion:      rec.PoolVersion,
			PoolID:           rec.PoolId,
			Exchange:         rec.Exchange,
			Token0Symbol:     rec.Token0Symbol,
			Token1Symbol:     rec.Token1Symbol,
			OpenedAt:         rec.OpenedAt.Format("2006-01-02 15:04:05"),
			OpenTxHash:       rec.OpenTxHash,
			OpenUSDTSpent:    amountToFloat(rec.OpenUSDTSpent, 18),
			OpenStableBefore: amountToFloat(rec.OpenStableBefore, 18),
			OpenStableAfter:  amountToFloat(rec.OpenStableAfter, 18),
			OpenGasSpentWei:  amountToFloat(rec.OpenGasSpentWei, 18),
			ProfitPct:        rec.ProfitPct,
			Status:           string(rec.Status),
			OpenTxURL:        explorerTxURLHelper(rec.Chain, rec.OpenTxHash),
		}
		if rec.ClosedAt != nil {
			row.ClosedAt = rec.ClosedAt.Format("2006-01-02 15:04:05")
			row.CloseTxHash = rec.CloseTxHash
			row.CloseUSDTRecv = amountToFloat(rec.CloseUSDTReceived, 18)
			row.CloseStableBefore = amountToFloat(rec.CloseStableBefore, 18)
			row.CloseStableAfter = amountToFloat(rec.CloseStableAfter, 18)
			row.CloseGasSpentWei = amountToFloat(rec.CloseGasSpentWei, 18)
			row.TotalGasUSDT = amountToFloat(rec.TotalGasUSDT, 18)
			row.ProfitUSDT = amountToFloat(rec.ProfitUSDT, 18)
			row.CloseTxURL = explorerTxURLHelper(rec.Chain, rec.CloseTxHash)
		}
		rows = append(rows, row)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tradeHistoryResponse{
		OK:      true,
		Total:   total,
		Records: rows,
	})
}

// handleTradeHistoryGET handles GET requests for trade history (for query param routing).
func (s *Server) handleTradeHistoryGET(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	initData := initDataFromQuery(r)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	limit := 20
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 50 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}

	db := database.DB.Model(&models.TradeRecord{}).Where("user_id = ?", user.ID)

	chain := strings.TrimSpace(r.URL.Query().Get("chain"))
	if chain != "" {
		db = db.Where("chain = ?", config.NormalizeChain(chain))
	}
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	if statusFilter != "" {
		db = db.Where("status = ?", statusFilter)
	}

	var total int64
	db.Count(&total)

	var records []models.TradeRecord
	if err := db.Order("opened_at DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		http.Error(w, "failed to query trade history", http.StatusInternalServerError)
		return
	}

	rows := make([]tradeRecordRow, 0, len(records))
	for _, rec := range records {
		row := tradeRecordRow{
			ID:               rec.ID,
			TaskID:           rec.TaskID,
			Chain:            rec.Chain,
			PoolVersion:      rec.PoolVersion,
			PoolID:           rec.PoolId,
			Exchange:         rec.Exchange,
			Token0Symbol:     rec.Token0Symbol,
			Token1Symbol:     rec.Token1Symbol,
			OpenedAt:         rec.OpenedAt.Format("2006-01-02 15:04:05"),
			OpenTxHash:       rec.OpenTxHash,
			OpenUSDTSpent:    amountToFloat(rec.OpenUSDTSpent, 18),
			OpenStableBefore: amountToFloat(rec.OpenStableBefore, 18),
			OpenStableAfter:  amountToFloat(rec.OpenStableAfter, 18),
			OpenGasSpentWei:  amountToFloat(rec.OpenGasSpentWei, 18),
			ProfitPct:        rec.ProfitPct,
			Status:           string(rec.Status),
			OpenTxURL:        explorerTxURLHelper(rec.Chain, rec.OpenTxHash),
		}
		if rec.ClosedAt != nil {
			row.ClosedAt = rec.ClosedAt.Format("2006-01-02 15:04:05")
			row.CloseTxHash = rec.CloseTxHash
			row.CloseUSDTRecv = amountToFloat(rec.CloseUSDTReceived, 18)
			row.CloseStableBefore = amountToFloat(rec.CloseStableBefore, 18)
			row.CloseStableAfter = amountToFloat(rec.CloseStableAfter, 18)
			row.CloseGasSpentWei = amountToFloat(rec.CloseGasSpentWei, 18)
			row.TotalGasUSDT = amountToFloat(rec.TotalGasUSDT, 18)
			row.ProfitUSDT = amountToFloat(rec.ProfitUSDT, 18)
			row.CloseTxURL = explorerTxURLHelper(rec.Chain, rec.CloseTxHash)
		}
		rows = append(rows, row)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tradeHistoryResponse{
		OK:      true,
		Total:   total,
		Records: rows,
	})
}
