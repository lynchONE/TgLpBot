package smart_money_golden_dog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"TgLpBot/base/clickhouse"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
	"TgLpBot/base/security"
	"TgLpBot/service/pool"
	userSvc "TgLpBot/service/user"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"gorm.io/gorm"
)

type clickHouseQueryer interface {
	Query(ctx context.Context, query string, args ...any) (driver.Rows, error)
}

type goldenDogPoolPositionRow struct {
	PoolVersion   string
	PoolID        string
	PositionCount uint64
}

type goldenDogPairAlert struct {
	AlertScope  string
	AlertKey    string
	PairLabel   string
	WalletCount uint64
}

type SmartMoneyGoldenDogService struct {
	ch      *clickhouse.ClickHouseService
	poolSvc *pool.PoolService
	cfgSvc  *userSvc.GlobalConfigService
	access  *userSvc.AccessService

	stopChan chan struct{}
	ticker   *time.Ticker
}

func NewSmartMoneyGoldenDogService(ch *clickhouse.ClickHouseService) *SmartMoneyGoldenDogService {
	return &SmartMoneyGoldenDogService{
		ch:       ch,
		poolSvc:  pool.NewPoolService(),
		cfgSvc:   userSvc.NewGlobalConfigService(),
		access:   userSvc.NewAccessService(),
		stopChan: make(chan struct{}),
		ticker:   time.NewTicker(30 * time.Second),
	}
}

func (s *SmartMoneyGoldenDogService) Start() {
	go s.runLoop()
}

func (s *SmartMoneyGoldenDogService) Stop() {
	if s == nil {
		return
	}
	select {
	case <-s.stopChan:
	default:
		close(s.stopChan)
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
}

func (s *SmartMoneyGoldenDogService) runLoop() {
	log.Println("[GoldenDog] service started")
	s.runOnce(time.Now())
	for {
		select {
		case <-s.stopChan:
			log.Println("[GoldenDog] service stopped")
			return
		case <-s.ticker.C:
			s.runOnce(time.Now())
		}
	}
}

func normalizeChain(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "bsc"
	}
	return v
}

func shortHex(value string, head int, tail int) string {
	s := strings.TrimSpace(value)
	if len(s) <= head+tail+2 {
		return s
	}
	return s[:head] + "..." + s[len(s)-tail:]
}

func clampGoldenDogMinWallets(v int) int {
	if v < 2 {
		return 2
	}
	if v > 100 {
		return 100
	}
	return v
}

func queryGoldenDogPoolPositions(ctx context.Context, conn clickHouseQueryer, chain string, windowMinutes int, limit int) ([]goldenDogPoolPositionRow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if conn == nil {
		return nil, fmt.Errorf("clickhouse not initialized")
	}
	chain = normalizeChain(chain)
	if windowMinutes <= 0 {
		windowMinutes = 10
	}
	if windowMinutes > 180 {
		windowMinutes = 180
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	startAt := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)

	q := `
		SELECT
			pool_version,
			pool_id,
			count() AS active_position_count
		FROM (
			SELECT
				pool_version,
				pool_id,
				contract_address,
				token_id,
				wallet_address,
				tick_lower,
				tick_upper,
				sum(
					if(
						pool_version = 'v4',
						toInt256OrZero(liquidity_delta),
						if(action = 'add', toInt256OrZero(liquidity_delta), -toInt256OrZero(liquidity_delta))
					)
				) AS net_liquidity
			FROM smart_lp_events
			WHERE ts >= ?
				AND lowerUTF8(chain) = ?
				AND action IN ('add', 'remove')
				AND pool_version != '' AND pool_id != ''
				AND wallet_address != ''
			GROUP BY pool_version, pool_id, contract_address, token_id, wallet_address, tick_lower, tick_upper
			HAVING net_liquidity > 0
		)
		GROUP BY pool_version, pool_id
		ORDER BY active_position_count DESC, pool_version ASC, pool_id ASC
		LIMIT ?
	`

	rows, err := conn.Query(ctx, q, startAt, chain, uint64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]goldenDogPoolPositionRow, 0, 32)
	for rows.Next() {
		var pv string
		var pid string
		var positionCount uint64
		if err := rows.Scan(&pv, &pid, &positionCount); err != nil {
			return nil, err
		}
		out = append(out, goldenDogPoolPositionRow{
			PoolVersion:   strings.ToLower(strings.TrimSpace(pv)),
			PoolID:        strings.ToLower(strings.TrimSpace(pid)),
			PositionCount: positionCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SmartMoneyGoldenDogService) loadBarkConfig(userID uint) (*notify.BarkConfig, error) {
	if userID == 0 {
		return nil, fmt.Errorf("invalid user")
	}
	if s == nil || s.cfgSvc == nil {
		return nil, fmt.Errorf("config service not initialized")
	}
	if config.AppConfig == nil {
		return nil, fmt.Errorf("app config not loaded")
	}

	userCfg, err := s.cfgSvc.GetOrCreate(userID)
	if err != nil || userCfg == nil {
		return nil, fmt.Errorf("load user global config failed")
	}
	if !userCfg.BarkEnabled || strings.TrimSpace(userCfg.BarkKeyEncrypted) == "" {
		return nil, nil
	}

	keyBytes, kerr := security.DecodeHexKey32(config.AppConfig.EncryptionKey)
	if kerr != nil {
		return nil, fmt.Errorf("decode encryption key failed")
	}
	plain, derr := security.DecryptAESGCMHex(keyBytes, userCfg.BarkKeyEncrypted)
	if derr != nil {
		return nil, fmt.Errorf("decrypt bark key failed")
	}
	barkKey := strings.TrimSpace(string(plain))
	if barkKey == "" {
		return nil, nil
	}

	cfg := &notify.BarkConfig{
		Server: userCfg.BarkServer,
		Key:    barkKey,
		Group:  userCfg.BarkGroup,
	}
	return cfg, nil
}

func (s *SmartMoneyGoldenDogService) loadPoolInfo(chain string, poolVersion string, poolID string) (*pool.PoolInfo, error) {
	if s == nil || s.poolSvc == nil {
		return nil, fmt.Errorf("pool service not initialized")
	}
	chain = normalizeChain(chain)
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if poolVersion == "" || poolID == "" {
		return nil, fmt.Errorf("pool not specified")
	}

	if poolVersion == "v4" {
		return s.poolSvc.GetV4PoolInfo(poolID)
	}
	return s.poolSvc.GetPoolInfoForChain(chain, poolID)
}

func poolPairLabelFromInfo(info *pool.PoolInfo) string {
	if info == nil {
		return ""
	}

	t0 := strings.TrimSpace(info.Token0Symbol)
	t1 := strings.TrimSpace(info.Token1Symbol)
	if t0 == "" && t1 == "" {
		return ""
	}
	if t0 == "" {
		return t1
	}
	if t1 == "" {
		return t0
	}
	return t0 + "/" + t1
}

func buildGoldenDogPairAlertKey(info *pool.PoolInfo) string {
	if info == nil {
		return ""
	}
	tokens := []string{
		strings.ToLower(strings.TrimSpace(info.Token0)),
		strings.ToLower(strings.TrimSpace(info.Token1)),
	}
	if tokens[0] == "" || tokens[1] == "" {
		return ""
	}
	sort.Strings(tokens)
	sum := sha256.Sum256([]byte(tokens[0] + ":" + tokens[1]))
	return hex.EncodeToString(sum[:])
}

func buildGoldenDogFallbackAlertKey(poolVersion string, poolID string) string {
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if poolVersion == "" || poolID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("pool|" + poolVersion + "|" + poolID))
	return hex.EncodeToString(sum[:])
}

func aggregateGoldenDogPairAlerts(
	chain string,
	rows []goldenDogPoolPositionRow,
	minWallets int,
	resolve func(string, string, string) (*pool.PoolInfo, error),
) []goldenDogPairAlert {
	type pairAggregate struct {
		pairLabel string
		count     int
	}

	minWallets = clampGoldenDogMinWallets(minWallets)
	groups := make(map[string]*pairAggregate)
	for _, row := range rows {
		var (
			pairLabel string
			alertKey  string
		)

		if resolve != nil {
			if info, err := resolve(chain, row.PoolVersion, row.PoolID); err == nil && info != nil {
				pairLabel = poolPairLabelFromInfo(info)
				alertKey = buildGoldenDogPairAlertKey(info)
			}
		}
		if alertKey == "" {
			alertKey = buildGoldenDogFallbackAlertKey(row.PoolVersion, row.PoolID)
		}
		if alertKey == "" {
			continue
		}
		if pairLabel == "" {
			pairLabel = shortHex(row.PoolID, 10, 8)
		}

		group := groups[alertKey]
		if group == nil {
			group = &pairAggregate{
				pairLabel: pairLabel,
				count:     0,
			}
			groups[alertKey] = group
		} else if strings.Contains(group.pairLabel, "...") && !strings.Contains(pairLabel, "...") {
			group.pairLabel = pairLabel
		}
		group.count += int(row.PositionCount)
	}

	out := make([]goldenDogPairAlert, 0, len(groups))
	for alertKey, group := range groups {
		if group.count < minWallets {
			continue
		}
		out = append(out, goldenDogPairAlert{
			AlertScope:  "pair",
			AlertKey:    alertKey,
			PairLabel:   group.pairLabel,
			WalletCount: uint64(group.count),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].WalletCount != out[j].WalletCount {
			return out[i].WalletCount > out[j].WalletCount
		}
		return out[i].PairLabel < out[j].PairLabel
	})
	return out
}

func (s *SmartMoneyGoldenDogService) shouldNotifyAlert(userID uint, chain string, alertScope string, alertKey string, cooldownMinutes int, now time.Time) (bool, error) {
	if database.DB == nil {
		return false, fmt.Errorf("db not initialized")
	}
	chain = normalizeChain(chain)
	alertScope = strings.ToLower(strings.TrimSpace(alertScope))
	alertKey = strings.ToLower(strings.TrimSpace(alertKey))
	if userID == 0 || alertScope == "" || alertKey == "" {
		return false, nil
	}
	if cooldownMinutes < 0 {
		cooldownMinutes = 0
	}
	if cooldownMinutes > 24*60 {
		cooldownMinutes = 24 * 60
	}

	var state models.SmartMoneyGoldenDogAlertState
	err := database.DB.Where("user_id = ? AND chain = ? AND pool_version = ? AND pool_id = ?", userID, chain, alertScope, alertKey).First(&state).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return false, err
	}
	if state.LastNotifiedAt == nil || state.LastNotifiedAt.IsZero() {
		return true, nil
	}
	if cooldownMinutes == 0 {
		return true, nil
	}
	return now.Sub(*state.LastNotifiedAt) >= time.Duration(cooldownMinutes)*time.Minute, nil
}

func (s *SmartMoneyGoldenDogService) markAlertNotified(userID uint, chain string, alertScope string, alertKey string, pair string, wallets uint64, now time.Time) {
	if database.DB == nil {
		return
	}
	chain = normalizeChain(chain)
	alertScope = strings.ToLower(strings.TrimSpace(alertScope))
	alertKey = strings.ToLower(strings.TrimSpace(alertKey))
	if userID == 0 || alertScope == "" || alertKey == "" {
		return
	}

	var state models.SmartMoneyGoldenDogAlertState
	err := database.DB.Where("user_id = ? AND chain = ? AND pool_version = ? AND pool_id = ?", userID, chain, alertScope, alertKey).First(&state).Error
	if err == nil {
		state.LastNotifiedAt = &now
		state.LastWallets = int(wallets)
		state.LastPair = strings.TrimSpace(pair)
		_ = database.DB.Save(&state).Error
		return
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}

	state = models.SmartMoneyGoldenDogAlertState{
		UserID:         userID,
		Chain:          chain,
		PoolVersion:    alertScope,
		PoolID:         alertKey,
		LastNotifiedAt: &now,
		LastWallets:    int(wallets),
		LastPair:       strings.TrimSpace(pair),
	}
	_ = database.DB.Create(&state).Error
}

func (s *SmartMoneyGoldenDogService) runOnce(now time.Time) {
	if s == nil || database.DB == nil {
		return
	}
	if s.ch == nil || s.ch.Conn == nil {
		return
	}

	var cfgs []models.SmartMoneyGoldenDogConfig
	if err := database.DB.Where("enabled = ?", true).Find(&cfgs).Error; err != nil {
		log.Printf("[GoldenDog] load configs failed: %v", err)
		return
	}
	if len(cfgs) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, cfg := range cfgs {
		if cfg.UserID == 0 || !cfg.Enabled {
			continue
		}

		if s.access != nil {
			check, err := s.access.CheckUserAccess(cfg.UserID, now)
			if err != nil || !check.Allowed {
				continue
			}
			if !check.IsAdmin {
				if check.Access == nil || !check.Access.SmartMoneyEnabled {
					continue
				}
			}
		}

		barkCfg, berr := s.loadBarkConfig(cfg.UserID)
		if berr != nil || barkCfg == nil {
			continue
		}

		poolRows, qerr := queryGoldenDogPoolPositions(ctx, s.ch.Conn, cfg.Chain, cfg.WindowMinutes, 200)
		if qerr != nil {
			continue
		}
		if len(poolRows) == 0 {
			continue
		}

		type poolInfoResult struct {
			info *pool.PoolInfo
			err  error
		}
		poolInfoCache := make(map[string]poolInfoResult)
		resolvePoolInfo := func(chain string, poolVersion string, poolID string) (*pool.PoolInfo, error) {
			cacheKey := normalizeChain(chain) + "|" + strings.ToLower(strings.TrimSpace(poolVersion)) + "|" + strings.ToLower(strings.TrimSpace(poolID))
			if cached, ok := poolInfoCache[cacheKey]; ok {
				return cached.info, cached.err
			}
			info, err := s.loadPoolInfo(chain, poolVersion, poolID)
			poolInfoCache[cacheKey] = poolInfoResult{info: info, err: err}
			return info, err
		}

		alerts := aggregateGoldenDogPairAlerts(cfg.Chain, poolRows, cfg.MinWallets, resolvePoolInfo)
		for _, alert := range alerts {
			ok, serr := s.shouldNotifyAlert(cfg.UserID, cfg.Chain, alert.AlertScope, alert.AlertKey, cfg.CooldownMinutes, now)
			if serr != nil || !ok {
				continue
			}

			pair := strings.TrimSpace(alert.PairLabel)
			if pair == "" {
				pair = "未知交易对"
			}

			title := "金狗通知"
			body := fmt.Sprintf("%s 当前有 %d 个活跃 LP 仓位，建议立即关注", pair, alert.WalletCount)
			if err := notify.SendBarkWithConfig(title, body, *barkCfg); err != nil {
				continue
			}

			s.markAlertNotified(cfg.UserID, cfg.Chain, alert.AlertScope, alert.AlertKey, pair, alert.WalletCount, now)
		}
	}
}
