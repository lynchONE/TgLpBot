package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
	smgd "TgLpBot/service/smart_money_golden_dog"
	userSvc "TgLpBot/service/user"
)

const (
	alphaReminderDefaultMinutes = 3
	alphaReminderMinMinutes     = 1
	alphaReminderMaxMinutes     = 120
	alphaReminderTickInterval   = 30 * time.Second
	alphaReminderTriggerWindow  = 90 * time.Second
	alphaReminderSentKeyLimit   = 50
)

type alphaReminderConfigResponse struct {
	OK                   bool                       `json:"ok"`
	Enabled              bool                       `json:"enabled"`
	ReminderMinutes      int                        `json:"reminder_minutes"`
	Intensity            string                     `json:"intensity"`
	BarkEnabled          bool                       `json:"bark_enabled"`
	BarkConfigured       bool                       `json:"bark_configured"`
	BarkReady            bool                       `json:"bark_ready"`
	AvailableIntensities []smgd.BarkIntensityOption `json:"available_intensities"`
}

type alphaReminderRequest struct {
	InitData        string `json:"initData"`
	Action          string `json:"action"`
	Enabled         *bool  `json:"enabled"`
	ReminderMinutes *int   `json:"reminder_minutes"`
	Intensity       string `json:"intensity"`
}

type alphaAirdropFeed struct {
	Airdrops []alphaAirdropItem `json:"airdrops"`
}

type alphaAirdropItem struct {
	Token  string `json:"token"`
	Name   string `json:"name"`
	Date   string `json:"date"`
	Time   string `json:"time"`
	Points string `json:"points"`
	Amount string `json:"amount"`
	Phase  int    `json:"phase"`
}

type AlphaAirdropReminderWorker struct {
	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	interval time.Duration
}

func NewAlphaAirdropReminderWorker() *AlphaAirdropReminderWorker {
	return &AlphaAirdropReminderWorker{interval: alphaReminderTickInterval}
}

func (w *AlphaAirdropReminderWorker) Start() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.running = true
	interval := w.interval
	w.mu.Unlock()

	go w.loop(ctx, interval)
}

func (w *AlphaAirdropReminderWorker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
	w.running = false
	w.cancel = nil
}

func (w *AlphaAirdropReminderWorker) loop(ctx context.Context, interval time.Duration) {
	w.runOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *AlphaAirdropReminderWorker) runOnce(ctx context.Context) {
	if database.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var configs []models.GlobalConfig
	if err := database.DB.WithContext(ctx).
		Where("alpha_airdrop_reminder_enabled = ?", true).
		Find(&configs).Error; err != nil {
		log.Printf("[AlphaReminder] load reminder configs failed: %v", err)
		return
	}
	if len(configs) == 0 {
		return
	}

	raw, err := loadAlphaData(ctx, alphaOverviewHTTPClient)
	if err != nil {
		log.Printf("[AlphaReminder] load alpha data failed: %v", err)
		return
	}
	airdrops, err := parseAlphaAirdrops(raw)
	if err != nil {
		log.Printf("[AlphaReminder] parse alpha data failed: %v", err)
		return
	}
	if len(airdrops) == 0 {
		return
	}

	for _, cfg := range configs {
		if err := processAlphaReminderConfig(ctx, cfg, airdrops, time.Now()); err != nil {
			log.Printf("[AlphaReminder] process user=%d failed: %v", cfg.UserID, err)
		}
	}
}

func parseAlphaAirdrops(raw json.RawMessage) ([]alphaAirdropItem, error) {
	var feed alphaAirdropFeed
	if err := json.Unmarshal(raw, &feed); err != nil {
		return nil, err
	}
	out := make([]alphaAirdropItem, 0, len(feed.Airdrops))
	for _, item := range feed.Airdrops {
		if strings.TrimSpace(item.Token) == "" && strings.TrimSpace(item.Name) == "" {
			continue
		}
		if strings.TrimSpace(item.Date) == "" || strings.TrimSpace(item.Time) == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func processAlphaReminderConfig(ctx context.Context, cfg models.GlobalConfig, airdrops []alphaAirdropItem, now time.Time) error {
	minutes := normalizeAlphaReminderMinutes(cfg.AlphaAirdropReminderMinutes)
	sent := parseAlphaReminderSentKeys(cfg.AlphaAirdropReminderSentKeys)
	status, err := smgd.ResolveUserBarkStatus(ctx, cfg.UserID)
	if err != nil {
		return fmt.Errorf("resolve bark: %w", err)
	}
	if !status.Ready {
		return nil
	}

	changed := false
	var firstSendErr error
	for _, item := range airdrops {
		eventTime, err := parseAlphaAirdropTime(item, now.Location())
		if err != nil {
			continue
		}
		notifyAt := eventTime.Add(-time.Duration(minutes) * time.Minute)
		if !alphaReminderDue(now, notifyAt) {
			continue
		}
		key := alphaReminderKey(item, eventTime)
		if sent[key] {
			continue
		}
		if err := sendAlphaAirdropBark(status.Config, cfg.AlphaAirdropReminderIntensity, item, eventTime, minutes); err != nil {
			if firstSendErr == nil {
				firstSendErr = fmt.Errorf("send bark token=%s: %w", strings.TrimSpace(item.Token), err)
			}
			continue
		}
		sent[key] = true
		changed = true
	}
	if changed {
		encoded := encodeAlphaReminderSentKeys(sent)
		if err := database.DB.WithContext(ctx).
			Model(&models.GlobalConfig{}).
			Where("user_id = ?", cfg.UserID).
			Update("alpha_airdrop_reminder_sent_keys", encoded).Error; err != nil {
			return err
		}
	}
	return firstSendErr
}

func parseAlphaAirdropTime(item alphaAirdropItem, loc *time.Location) (time.Time, error) {
	date := strings.TrimSpace(item.Date)
	clock := strings.TrimSpace(item.Time)
	if loc == nil {
		loc = time.Local
	}
	return time.ParseInLocation("2006-01-02 15:04", date+" "+clock, loc)
}

func alphaReminderDue(now time.Time, notifyAt time.Time) bool {
	return !now.Before(notifyAt) && now.Sub(notifyAt) <= alphaReminderTriggerWindow
}

func alphaReminderKey(item alphaAirdropItem, eventTime time.Time) string {
	token := strings.ToUpper(strings.TrimSpace(item.Token))
	name := strings.TrimSpace(item.Name)
	return strings.Join([]string{token, name, eventTime.Format("2006-01-02T15:04")}, "|")
}

func sendAlphaAirdropBark(base notify.BarkConfig, intensity string, item alphaAirdropItem, eventTime time.Time, minutes int) error {
	token := strings.ToUpper(strings.TrimSpace(item.Token))
	name := strings.TrimSpace(item.Name)
	title := "Alpha 空投提醒"
	target := token
	if target == "" {
		target = name
	}
	bodyParts := []string{target}
	if name != "" && !strings.EqualFold(name, target) {
		bodyParts = append(bodyParts, name)
	}
	bodyParts = append(bodyParts, fmt.Sprintf("%s 开始，提前 %d 分钟提醒", eventTime.Format("01-02 15:04"), minutes))
	if amount := strings.TrimSpace(item.Amount); amount != "" {
		bodyParts = append(bodyParts, "数量 "+amount)
	}
	if points := strings.TrimSpace(item.Points); points != "" {
		bodyParts = append(bodyParts, "积分 "+points)
	}
	return notify.SendBarkWithConfig(title, strings.Join(bodyParts, " · "), smgd.BarkConfigForIntensity(base, intensity))
}

func normalizeAlphaReminderMinutes(value int) int {
	if value < alphaReminderMinMinutes {
		return alphaReminderDefaultMinutes
	}
	if value > alphaReminderMaxMinutes {
		return alphaReminderMaxMinutes
	}
	return value
}

func parseAlphaReminderSentKeys(raw string) map[string]bool {
	out := map[string]bool{}
	text := strings.TrimSpace(raw)
	if text == "" {
		return out
	}
	var keys []string
	if err := json.Unmarshal([]byte(text), &keys); err != nil {
		return out
	}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func encodeAlphaReminderSentKeys(values map[string]bool) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > alphaReminderSentKeyLimit {
		keys = keys[len(keys)-alphaReminderSentKeyLimit:]
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *Server) handleAlphaReminder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req alphaReminderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, status, msg := authenticateTelegramWebAppUser(strings.TrimSpace(req.InitData))
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
	if status, msg := requireModulePermission(check, models.AccessModuleAssets); status != 0 {
		http.Error(w, msg, status)
		return
	}

	cfgService := userSvc.NewGlobalConfigService()
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "save" {
		updates := map[string]interface{}{}
		if req.Enabled != nil {
			updates["alpha_airdrop_reminder_enabled"] = *req.Enabled
		}
		if req.ReminderMinutes != nil {
			updates["alpha_airdrop_reminder_minutes"] = normalizeAlphaReminderMinutes(*req.ReminderMinutes)
		}
		if strings.TrimSpace(req.Intensity) != "" {
			updates["alpha_airdrop_reminder_intensity"] = smgd.NormalizeBarkIntensity(req.Intensity)
		}
		if len(updates) > 0 {
			if _, err := cfgService.Update(user.ID, updates); err != nil {
				http.Error(w, "failed to save alpha reminder", http.StatusInternalServerError)
				return
			}
		}
	}

	cfg, err := cfgService.GetOrCreate(user.ID)
	if err != nil {
		http.Error(w, "failed to load alpha reminder", http.StatusInternalServerError)
		return
	}
	barkStatus, err := smgd.ResolveUserBarkStatus(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to load bark status", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, alphaReminderConfigResponse{
		OK:                   true,
		Enabled:              cfg.AlphaAirdropReminderEnabled,
		ReminderMinutes:      normalizeAlphaReminderMinutes(cfg.AlphaAirdropReminderMinutes),
		Intensity:            smgd.NormalizeBarkIntensity(cfg.AlphaAirdropReminderIntensity),
		BarkEnabled:          barkStatus.Enabled,
		BarkConfigured:       barkStatus.Configured,
		BarkReady:            barkStatus.Ready,
		AvailableIntensities: smgd.BarkIntensityOptions(),
	})
}
