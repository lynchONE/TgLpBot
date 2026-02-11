package smart_money_golden_dog

import (
	"context"
	"errors"
	"fmt"
	"log"
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

type goldenDogPoolRow struct {
	PoolVersion string
	PoolID      string
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

func queryGoldenDogPools(ctx context.Context, conn clickHouseQueryer, chain string, windowMinutes int, minWallets int, limit int) ([]goldenDogPoolRow, error) {
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
	if minWallets < 2 {
		minWallets = 2
	}
	if minWallets > 100 {
		minWallets = 100
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	startAt := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)

	q := `
		SELECT
			pool_version,
			pool_id,
			uniqExact(wallet_address) AS wallet_count
		FROM smart_lp_events
		WHERE ts >= ?
			AND lowerUTF8(chain) = ?
			AND action = 'add'
			AND pool_version != '' AND pool_id != ''
		GROUP BY pool_version, pool_id
		HAVING wallet_count >= ?
		ORDER BY wallet_count DESC
		LIMIT ?
	`

	rows, err := conn.Query(ctx, q, startAt, chain, uint64(minWallets), uint64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]goldenDogPoolRow, 0, 16)
	for rows.Next() {
		var pv string
		var pid string
		var cnt uint64
		if err := rows.Scan(&pv, &pid, &cnt); err != nil {
			return nil, err
		}
		out = append(out, goldenDogPoolRow{
			PoolVersion: strings.ToLower(strings.TrimSpace(pv)),
			PoolID:      strings.ToLower(strings.TrimSpace(pid)),
			WalletCount: cnt,
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

func (s *SmartMoneyGoldenDogService) poolPairLabel(poolVersion string, poolID string) string {
	if s == nil || s.poolSvc == nil {
		return ""
	}
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if poolVersion == "" || poolID == "" {
		return ""
	}

	var info *pool.PoolInfo
	var err error
	if poolVersion == "v4" {
		info, err = s.poolSvc.GetV4PoolInfo(poolID)
	} else {
		info, err = s.poolSvc.GetPoolInfo(poolID)
	}
	if err != nil || info == nil {
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

func (s *SmartMoneyGoldenDogService) shouldNotify(userID uint, chain string, poolVersion string, poolID string, cooldownMinutes int, now time.Time) (bool, error) {
	if database.DB == nil {
		return false, fmt.Errorf("db not initialized")
	}
	chain = normalizeChain(chain)
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if userID == 0 || poolVersion == "" || poolID == "" {
		return false, nil
	}
	if cooldownMinutes < 0 {
		cooldownMinutes = 0
	}
	if cooldownMinutes > 24*60 {
		cooldownMinutes = 24 * 60
	}

	var state models.SmartMoneyGoldenDogAlertState
	err := database.DB.Where("user_id = ? AND chain = ? AND pool_version = ? AND pool_id = ?", userID, chain, poolVersion, poolID).First(&state).Error
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

func (s *SmartMoneyGoldenDogService) markNotified(userID uint, chain string, poolVersion string, poolID string, pair string, wallets uint64, now time.Time) {
	if database.DB == nil {
		return
	}
	chain = normalizeChain(chain)
	poolVersion = strings.ToLower(strings.TrimSpace(poolVersion))
	poolID = strings.ToLower(strings.TrimSpace(poolID))
	if userID == 0 || poolVersion == "" || poolID == "" {
		return
	}

	var state models.SmartMoneyGoldenDogAlertState
	err := database.DB.Where("user_id = ? AND chain = ? AND pool_version = ? AND pool_id = ?", userID, chain, poolVersion, poolID).First(&state).Error
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
		PoolVersion:    poolVersion,
		PoolID:         poolID,
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

		pools, qerr := queryGoldenDogPools(ctx, s.ch.Conn, cfg.Chain, cfg.WindowMinutes, cfg.MinWallets, 50)
		if qerr != nil {
			continue
		}
		if len(pools) == 0 {
			continue
		}

		for _, row := range pools {
			ok, serr := s.shouldNotify(cfg.UserID, cfg.Chain, row.PoolVersion, row.PoolID, cfg.CooldownMinutes, now)
			if serr != nil || !ok {
				continue
			}

			pair := s.poolPairLabel(row.PoolVersion, row.PoolID)
			if pair == "" {
				pair = shortHex(row.PoolID, 10, 8)
			}

			title := "金狗通知"
			body := fmt.Sprintf("%s 已有 %d 个钱包在加LP，建议立即关注", pair, row.WalletCount)
			if err := notify.SendBarkWithConfig(title, body, *barkCfg); err != nil {
				continue
			}

			s.markNotified(cfg.UserID, cfg.Chain, row.PoolVersion, row.PoolID, pair, row.WalletCount, now)
		}
	}
}
