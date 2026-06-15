package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/rpcpool"
	"TgLpBot/service/pool_sync"
	userSvc "TgLpBot/service/user"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type adminPoolDataSourceRequest struct {
	InitData string `json:"initData"`
	Action   string `json:"action"`

	SourceID uint `json:"source_id,omitempty"`

	Name             string            `json:"name,omitempty"`
	SourceType       string            `json:"source_type,omitempty"`
	Chain            string            `json:"chain,omitempty"`
	TimeframeMinutes int               `json:"timeframe_minutes,omitempty"`
	Limit            int               `json:"limit,omitempty"`
	BaseURL          string            `json:"base_url,omitempty"`
	PathTemplate     string            `json:"path_template,omitempty"`
	QueryTemplate    map[string]string `json:"query_template,omitempty"`
	Protocols        []string          `json:"protocols,omitempty"`
	Dexes            []string          `json:"dexes,omitempty"`
	SetCurrent       bool              `json:"set_current,omitempty"`

	fields map[string]json.RawMessage `json:"-"`
}

func (req adminPoolDataSourceRequest) hasField(name string) bool {
	_, ok := req.fields[name]
	return ok
}

func (req adminPoolDataSourceRequest) poolDataSourceInput() pool_sync.PoolDataSourceInput {
	return pool_sync.PoolDataSourceInput{
		Name:             strings.TrimSpace(req.Name),
		NameSet:          req.hasField("name"),
		SourceType:       strings.TrimSpace(req.SourceType),
		Chain:            strings.TrimSpace(req.Chain),
		TimeframeMinutes: req.TimeframeMinutes,
		Limit:            req.Limit,
		BaseURL:          strings.TrimSpace(req.BaseURL),
		PathTemplate:     strings.TrimSpace(req.PathTemplate),
		PathTemplateSet:  req.hasField("path_template"),
		QueryTemplate:    req.QueryTemplate,
		Protocols:        req.Protocols,
		Dexes:            req.Dexes,
		SetCurrent:       req.SetCurrent,
	}
}

func (req *adminPoolDataSourceRequest) UnmarshalJSON(data []byte) error {
	type alias adminPoolDataSourceRequest
	allowed := map[string]struct{}{
		"initData":          {},
		"action":            {},
		"source_id":         {},
		"name":              {},
		"source_type":       {},
		"chain":             {},
		"timeframe_minutes": {},
		"limit":             {},
		"base_url":          {},
		"path_template":     {},
		"query_template":    {},
		"protocols":         {},
		"dexes":             {},
		"set_current":       {},
	}
	var parsed alias
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key := range fields {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("json: unknown field %q", key)
		}
	}
	*req = adminPoolDataSourceRequest(parsed)
	req.fields = fields
	return nil
}

func (req adminPoolDataSourceRequest) MarshalJSON() ([]byte, error) {
	type alias adminPoolDataSourceRequest
	return json.Marshal(alias(req))
}

type adminPoolDataSourceDTO struct {
	ID uint `json:"id,omitempty"`

	Name             string            `json:"name"`
	SourceType       string            `json:"source_type"`
	Chain            string            `json:"chain"`
	TimeframeMinutes int               `json:"timeframe_minutes"`
	Limit            int               `json:"limit"`
	BaseURL          string            `json:"base_url"`
	BaseURLMasked    string            `json:"base_url_masked"`
	PathTemplate     string            `json:"path_template,omitempty"`
	QueryTemplate    map[string]string `json:"query_template,omitempty"`
	Protocols        []string          `json:"protocols,omitempty"`
	Dexes            []string          `json:"dexes,omitempty"`

	IsCurrent     bool `json:"is_current"`
	IsEnabled     bool `json:"is_enabled"`
	IsEnvFallback bool `json:"is_env_fallback,omitempty"`

	LastCheckedAt     *time.Time      `json:"last_checked_at,omitempty"`
	LastSuccessAt     *time.Time      `json:"last_success_at,omitempty"`
	LastLatencyMs     int64           `json:"last_latency_ms"`
	LastError         string          `json:"last_error,omitempty"`
	LastFieldCoverage json.RawMessage `json:"last_field_coverage,omitempty"`
}

type adminPoolDataSourceGroup struct {
	Chain            string                   `json:"chain"`
	TimeframeMinutes int                      `json:"timeframe_minutes"`
	EffectiveSource  *adminPoolDataSourceDTO  `json:"effective_source,omitempty"`
	EnvFallback      adminPoolDataSourceDTO   `json:"env_fallback"`
	Sources          []adminPoolDataSourceDTO `json:"sources"`
}

type adminPoolDataSourcesResponse struct {
	OK     bool                       `json:"ok"`
	Now    time.Time                  `json:"now"`
	Groups []adminPoolDataSourceGroup `json:"groups"`
}

var (
	adminPoolDataSourcesGetOrCreateUser = func(parsed *TelegramWebAppInitData) (*models.User, error) {
		userService := userSvc.NewUserService()
		return userService.GetOrCreateUser(
			parsed.User.ID,
			parsed.User.Username,
			parsed.User.FirstName,
			parsed.User.LastName,
			parsed.User.LanguageCode,
		)
	}
	adminPoolDataSourcesIsAdminUser = func(userID uint) bool {
		accessService := userSvc.NewAccessService()
		return accessService.IsAdminUser(userID)
	}
	adminPoolDataSourcesManager = pool_sync.DefaultPoolDataSourceManager
)

func (s *Server) handleAdminPoolDataSources(w http.ResponseWriter, r *http.Request) {
	initData := ""
	action := ""
	var req adminPoolDataSourceRequest

	switch r.Method {
	case http.MethodGet:
		initData = strings.TrimSpace(r.URL.Query().Get("initData"))
		if initData == "" {
			initData = strings.TrimSpace(r.URL.Query().Get("init_data"))
		}
		action = "list"
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		initData = strings.TrimSpace(req.InitData)
		action = strings.ToLower(strings.TrimSpace(req.Action))
		if action == "" {
			action = "list"
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}
	parsed, err := ParseTelegramWebAppInitData(initData, config.AppConfig.TelegramBotToken)
	if err != nil {
		if errors.Is(err, ErrMissingInitData) {
			http.Error(w, "missing initData", http.StatusBadRequest)
		} else {
			http.Error(w, "invalid initData", http.StatusUnauthorized)
		}
		return
	}

	user, err := adminPoolDataSourcesGetOrCreateUser(parsed)
	if err != nil {
		http.Error(w, "load user failed", http.StatusInternalServerError)
		return
	}
	if !adminPoolDataSourcesIsAdminUser(user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	mgr := adminPoolDataSourcesManager()
	ctx := r.Context()

	switch action {
	case "list":
		writePoolDataSourcesList(w, ctx, mgr)
	case "add":
		_, err := mgr.AddSource(ctx, poolDataSourceInputFromAdminRequest(req))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writePoolDataSourcesList(w, ctx, mgr)
	case "update":
		if req.SourceID == 0 {
			http.Error(w, "source_id required", http.StatusBadRequest)
			return
		}
		if _, err := mgr.UpdateSource(ctx, req.SourceID, poolDataSourceInputFromAdminRequest(req)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writePoolDataSourcesList(w, ctx, mgr)
	case "switch":
		if req.SourceID == 0 {
			http.Error(w, "source_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.SwitchCurrent(ctx, req.SourceID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writePoolDataSourcesList(w, ctx, mgr)
	case "enable":
		if req.SourceID == 0 {
			http.Error(w, "source_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.EnableSource(ctx, req.SourceID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writePoolDataSourcesList(w, ctx, mgr)
	case "disable":
		if req.SourceID == 0 {
			http.Error(w, "source_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.DisableSource(ctx, req.SourceID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writePoolDataSourcesList(w, ctx, mgr)
	case "delete":
		if req.SourceID == 0 {
			http.Error(w, "source_id required", http.StatusBadRequest)
			return
		}
		if err := mgr.DeleteSource(ctx, req.SourceID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writePoolDataSourcesList(w, ctx, mgr)
	case "check":
		if req.SourceID == 0 {
			http.Error(w, "source_id required", http.StatusBadRequest)
			return
		}
		if _, err := mgr.CheckSource(ctx, req.SourceID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writePoolDataSourcesList(w, ctx, mgr)
	default:
		http.Error(w, "invalid action", http.StatusBadRequest)
	}
}

func poolDataSourceInputFromAdminRequest(req adminPoolDataSourceRequest) pool_sync.PoolDataSourceInput {
	return req.poolDataSourceInput()
}

func writePoolDataSourcesList(w http.ResponseWriter, ctx context.Context, mgr *pool_sync.PoolDataSourceManager) {
	type groupKey struct {
		chain     string
		timeframe int
	}

	now := time.Now()
	rows, _ := mgr.ListAll(ctx)
	keys := map[groupKey]struct{}{{chain: "bsc", timeframe: 5}: {}}
	for _, row := range rows {
		chain := strings.TrimSpace(row.Chain)
		if chain == "" {
			chain = "bsc"
		}
		tf := row.TimeframeMinutes
		if tf <= 0 {
			tf = 5
		}
		keys[groupKey{chain: chain, timeframe: tf}] = struct{}{}
	}

	groups := make([]adminPoolDataSourceGroup, 0, len(keys))
	for key := range keys {
		env := adminPoolSourceDTOFromConfig(pool_sync.PoolDataSourceConfig{
			Name:             "PoolM (.env)",
			SourceType:       pool_sync.PoolDataSourceTypePoolMTopFees,
			Chain:            key.chain,
			TimeframeMinutes: key.timeframe,
			Limit:            100,
			BaseURL:          poolDataSourceEnvBaseURL(),
			IsEnabled:        true,
			IsEnvFallback:    true,
		})
		candidates := mgr.CandidateSources(ctx, key.chain, key.timeframe)
		var effective *adminPoolDataSourceDTO
		if len(candidates) > 0 {
			dto := adminPoolSourceDTOFromConfig(candidates[0])
			effective = &dto
			if candidates[0].IsEnvFallback {
				env = dto
			}
		}

		items := make([]adminPoolDataSourceDTO, 0)
		for _, row := range rows {
			tf := row.TimeframeMinutes
			if tf <= 0 {
				tf = 5
			}
			if row.Chain != key.chain || tf != key.timeframe {
				continue
			}
			items = append(items, adminPoolSourceDTOFromModel(row))
		}

		groups = append(groups, adminPoolDataSourceGroup{
			Chain:            key.chain,
			TimeframeMinutes: key.timeframe,
			EffectiveSource:  effective,
			EnvFallback:      env,
			Sources:          items,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Chain != groups[j].Chain {
			return groups[i].Chain < groups[j].Chain
		}
		return groups[i].TimeframeMinutes < groups[j].TimeframeMinutes
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(adminPoolDataSourcesResponse{
		OK:     true,
		Now:    now,
		Groups: groups,
	})
}

func adminPoolSourceDTOFromModel(source models.PoolDataSource) adminPoolDataSourceDTO {
	coverage := json.RawMessage("{}")
	if raw := strings.TrimSpace(source.LastFieldCoverageJSON); raw != "" && json.Valid([]byte(raw)) {
		coverage = json.RawMessage(raw)
	}
	return adminPoolDataSourceDTO{
		ID:                source.ID,
		Name:              strings.TrimSpace(source.Name),
		SourceType:        pool_sync.NormalizePoolDataSourceType(source.SourceType),
		Chain:             strings.TrimSpace(source.Chain),
		TimeframeMinutes:  source.TimeframeMinutes,
		Limit:             source.Limit,
		BaseURL:           strings.TrimSpace(source.BaseURL),
		BaseURLMasked:     rpcpool.MaskURL(source.BaseURL),
		PathTemplate:      strings.TrimSpace(source.PathTemplate),
		QueryTemplate:     poolDataSourceMapFromJSON(source.QueryTemplateJSON),
		Protocols:         poolDataSourceStringListFromJSON(source.ProtocolsJSON),
		Dexes:             poolDataSourceStringListFromJSON(source.DexesJSON),
		IsCurrent:         source.IsCurrent,
		IsEnabled:         source.IsEnabled,
		LastCheckedAt:     source.LastCheckedAt,
		LastSuccessAt:     source.LastSuccessAt,
		LastLatencyMs:     source.LastLatencyMs,
		LastError:         strings.TrimSpace(source.LastError),
		LastFieldCoverage: coverage,
	}
}

func adminPoolSourceDTOFromConfig(source pool_sync.PoolDataSourceConfig) adminPoolDataSourceDTO {
	id := uint(0)
	if source.ID != nil {
		id = *source.ID
	}
	return adminPoolDataSourceDTO{
		ID:               id,
		Name:             strings.TrimSpace(source.Name),
		SourceType:       pool_sync.NormalizePoolDataSourceType(source.SourceType),
		Chain:            strings.TrimSpace(source.Chain),
		TimeframeMinutes: source.TimeframeMinutes,
		Limit:            source.Limit,
		BaseURL:          strings.TrimSpace(source.BaseURL),
		BaseURLMasked:    rpcpool.MaskURL(source.BaseURL),
		PathTemplate:     strings.TrimSpace(source.PathTemplate),
		QueryTemplate:    source.QueryTemplate,
		Protocols:        source.Protocols,
		Dexes:            source.Dexes,
		IsCurrent:        source.IsCurrent,
		IsEnabled:        source.IsEnabled,
		IsEnvFallback:    source.IsEnvFallback,
	}
}

func poolDataSourceEnvBaseURL() string {
	if config.AppConfig != nil && strings.TrimSpace(config.AppConfig.PoolsSyncPoolMBaseURL) != "" {
		return strings.TrimSpace(config.AppConfig.PoolsSyncPoolMBaseURL)
	}
	return "https://mapi.poolm.xyz"
}

func poolDataSourceStringListFromJSON(raw string) []string {
	var out []string
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func poolDataSourceMapFromJSON(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}
