package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/smart_lp"
	"TgLpBot/service/ws"
	"encoding/json"
	"log"
	"strings"
	"time"

	userSvc "TgLpBot/service/user"
)

// SetWSHub injects the WebSocket hub so the bot can broadcast messages to miniapp clients.
func (b *Bot) SetWSHub(hub *ws.Hub) {
	b.wsHub = hub
}

// smartMoneyExitMessage is the JSON payload sent via WebSocket.
type smartMoneyExitMessage struct {
	Type string                `json:"type"`
	Data smartMoneyExitPayload `json:"data"`
}

type smartMoneyExitPayload struct {
	Wallet       string `json:"wallet"`
	PoolID       string `json:"pool_id"`
	PoolVersion  string `json:"pool_version"`
	Chain        string `json:"chain"`
	Token0Symbol string `json:"token0_symbol"`
	Token1Symbol string `json:"token1_symbol"`
	Amount0      string `json:"amount0"`
	Amount1      string `json:"amount1"`
	TickLower    int    `json:"tick_lower"`
	TickUpper    int    `json:"tick_upper"`
	Timestamp    string `json:"timestamp"`
}

// broadcastSmartMoneyExit is called when a smart money remove event is detected.
// It finds all users with running tasks on the same pool and pushes a notification via WebSocket.
func (b *Bot) broadcastSmartMoneyExit(ev smart_lp.SmartLPRemoveEvent) {
	if b.wsHub == nil || database.DB == nil {
		return
	}

	chain := strings.ToLower(strings.TrimSpace(ev.Chain))
	poolID := strings.ToLower(strings.TrimSpace(ev.PoolID))
	poolVersion := strings.ToLower(strings.TrimSpace(ev.PoolVersion))
	if chain == "" || poolID == "" {
		return
	}

	// Find all users with running/waiting tasks on this pool.
	type userRow struct {
		UserID       uint   `gorm:"column:user_id"`
		Token0Symbol string `gorm:"column:token0_symbol"`
		Token1Symbol string `gorm:"column:token1_symbol"`
	}
	var rows []userRow
	query := database.DB.Table("strategy_tasks").
		Select("DISTINCT user_id, token0_symbol, token1_symbol").
		Where("pool_id = ? AND chain = ? AND status IN ? AND paused = ?",
			poolID, chain,
			[]models.StrategyStatus{models.StrategyStatusRunning, models.StrategyStatusWaiting},
			false,
		)
	if poolVersion != "" {
		query = query.Where("pool_version = ?", poolVersion)
	}
	if err := query.Scan(&rows).Error; err != nil {
		log.Printf("[SmartMoneyNotify] query tasks failed: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	// Deduplicate user IDs and grab token symbols from first match.
	token0 := ""
	token1 := ""
	userIDSet := make(map[uint]struct{})
	for _, r := range rows {
		userIDSet[r.UserID] = struct{}{}
		if token0 == "" {
			token0 = r.Token0Symbol
			token1 = r.Token1Symbol
		}
	}

	// Filter by global config: only users with SmartMoneyExitNotifyEnabled.
	cfgSvc := userSvc.NewGlobalConfigService()
	var targetUserIDs []uint
	for uid := range userIDSet {
		cfg, err := cfgSvc.GetOrCreate(uid)
		if err != nil {
			continue
		}
		if cfg.SmartMoneyExitNotifyEnabled {
			targetUserIDs = append(targetUserIDs, uid)
		}
	}
	if len(targetUserIDs) == 0 {
		return
	}

	ts := ev.Ts
	if ts.IsZero() {
		ts = time.Now()
	}

	msg := smartMoneyExitMessage{
		Type: "smart_money_exit",
		Data: smartMoneyExitPayload{
			Wallet:       ev.WalletAddress,
			PoolID:       poolID,
			PoolVersion:  poolVersion,
			Chain:        chain,
			Token0Symbol: token0,
			Token1Symbol: token1,
			Amount0:      ev.Amount0,
			Amount1:      ev.Amount1,
			TickLower:    ev.TickLower,
			TickUpper:    ev.TickUpper,
			Timestamp:    ts.UTC().Format(time.RFC3339),
		},
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[SmartMoneyNotify] marshal failed: %v", err)
		return
	}

	b.wsHub.SendToUsers(targetUserIDs, payload)
	log.Printf("[SmartMoneyNotify] pushed smart_money_exit pool=%s to %d users", shortPoolID(poolID), len(targetUserIDs))
}

func shortPoolID(v string) string {
	if len(v) <= 18 {
		return v
	}
	return v[:10] + "..." + v[len(v)-6:]
}
