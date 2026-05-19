package web_server

import (
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
	sm "TgLpBot/service/smart_money"
	smgd "TgLpBot/service/smart_money_golden_dog"
	smwoa "TgLpBot/service/smart_money_watch_open_alert"
	"encoding/json"
	"net/http"
	"strings"
)

type smartMoneyWatchWalletItem struct {
	WalletAddress        string  `json:"wallet_address"`
	WalletLabel          *string `json:"wallet_label,omitempty"`
	WalletAvatar         *string `json:"wallet_avatar_url,omitempty"`
	WalletSource         string  `json:"wallet_source,omitempty"`
	WalletSourceContract string  `json:"wallet_source_contract,omitempty"`
	WalletColor          string  `json:"wallet_color"`
}

type smartMoneyWatchWalletsEnvelope struct {
	OK      bool                        `json:"ok"`
	Chain   string                      `json:"chain"`
	Count   int                         `json:"count"`
	Wallets []string                    `json:"wallets"`
	Items   []smartMoneyWatchWalletItem `json:"items"`
}

type smartMoneyWatchWalletsUpdateRequest struct {
	InitData      string   `json:"initData"`
	Chain         string   `json:"chain"`
	WalletAddress string   `json:"wallet_address"`
	Watched       *bool    `json:"watched"`
	Wallets       []string `json:"wallets"`
}

type smartMoneyWatchOpenAlertConfigEnvelope struct {
	OK             bool                                   `json:"ok"`
	Config         *models.SmartMoneyWatchOpenAlertConfig `json:"config,omitempty"`
	BarkEnabled    bool                                   `json:"bark_enabled"`
	BarkConfigured bool                                   `json:"bark_configured"`
	BarkReady      bool                                   `json:"bark_ready"`
}

type smartMoneyWatchOpenAlertUpdateRequest struct {
	InitData     string `json:"initData"`
	Chain        string `json:"chain"`
	Enabled      *bool  `json:"enabled"`
	BarkEnabled  *bool  `json:"bark_enabled"`
	SoundEnabled *bool  `json:"sound_enabled"`
}

type smartMoneyWatchOpenAlertTestRequest struct {
	InitData string `json:"initData"`
	Chain    string `json:"chain"`
}

type smartMoneyWatchOpenAlertMessageEnvelope struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

func (s *Server) handleSmartMoneyWatchWallets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSmartMoneyWatchWallets(w, r)
	case http.MethodPost:
		s.handlePostSmartMoneyWatchWallets(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetSmartMoneyWatchWallets(w http.ResponseWriter, r *http.Request) {
	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, initDataFromQuery(r))
	if !ok {
		return
	}
	s.writeSmartMoneyWatchWalletsEnvelope(w, r, user.ID, normalizeSmartMoneyGoldenDogChain(r.URL.Query().Get("chain")))
}

func (s *Server) handlePostSmartMoneyWatchWallets(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

	var req smartMoneyWatchWalletsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, strings.TrimSpace(req.InitData))
	if !ok {
		return
	}

	chain := normalizeSmartMoneyGoldenDogChain(req.Chain)
	repo := smwoa.NewRepository()

	if req.Wallets != nil {
		normalized, invalid := normalizeSmartMoneyWatchWalletInput(req.Wallets)
		if invalid {
			http.Error(w, "invalid watch wallet list", http.StatusBadRequest)
			return
		}
		if err := repo.ReplaceWatchWallets(r.Context(), user.ID, chain, normalized); err != nil {
			http.Error(w, "failed to save watch wallets", http.StatusInternalServerError)
			return
		}
	} else {
		walletAddress := strings.TrimSpace(req.WalletAddress)
		if walletAddress == "" {
			http.Error(w, "wallet_address is required", http.StatusBadRequest)
			return
		}
		watched := false
		if req.Watched != nil {
			watched = *req.Watched
		} else {
			row, err := repo.GetWatchWallet(r.Context(), user.ID, chain, walletAddress)
			if err != nil {
				http.Error(w, "failed to load watch wallet", http.StatusInternalServerError)
				return
			}
			watched = row == nil
		}
		if err := repo.SetWatchWallet(r.Context(), user.ID, chain, walletAddress, watched); err != nil {
			http.Error(w, "failed to update watch wallet", http.StatusBadRequest)
			return
		}
	}

	s.writeSmartMoneyWatchWalletsEnvelope(w, r, user.ID, chain)
}

func (s *Server) writeSmartMoneyWatchWalletsEnvelope(w http.ResponseWriter, r *http.Request, userID uint, chain string) {
	repo := smwoa.NewRepository()
	rows, err := repo.ListWatchWallets(r.Context(), userID, chain)
	if err != nil {
		http.Error(w, "failed to load watch wallets", http.StatusInternalServerError)
		return
	}

	items := make([]smartMoneyWatchWalletItem, 0, len(rows))
	wallets := make([]string, 0, len(rows))
	smRepo := sm.NewRepository()
	chainID := smartMoneyWatchWalletChainID(chain)

	for _, row := range rows {
		wallets = append(wallets, row.WalletAddress)
		item := smartMoneyWatchWalletItem{
			WalletAddress: row.WalletAddress,
			WalletColor:   sm.WalletColor(row.WalletAddress),
		}
		wallet, walletErr := smRepo.GetMonitoredWalletByAddress(r.Context(), row.WalletAddress, chainID)
		if walletErr == nil && wallet != nil {
			item.WalletLabel = wallet.Label
			item.WalletAvatar = wallet.AvatarURL
			item.WalletSource = smartMoneyWalletSourceValue(wallet)
			item.WalletSourceContract = smartMoneyWalletSourceContractValue(wallet)
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, smartMoneyWatchWalletsEnvelope{
		OK:      true,
		Chain:   chain,
		Count:   len(wallets),
		Wallets: wallets,
		Items:   items,
	})
}

func (s *Server) handleSmartMoneyWatchOpenAlertConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetSmartMoneyWatchOpenAlertConfig(w, r)
	case http.MethodPost:
		s.handlePostSmartMoneyWatchOpenAlertConfig(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetSmartMoneyWatchOpenAlertConfig(w http.ResponseWriter, r *http.Request) {
	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, initDataFromQuery(r))
	if !ok {
		return
	}

	repo := smwoa.NewRepository()
	cfg, err := repo.GetOrCreateConfig(r.Context(), user.ID, normalizeSmartMoneyGoldenDogChain(r.URL.Query().Get("chain")))
	if err != nil {
		http.Error(w, "failed to load config", http.StatusInternalServerError)
		return
	}
	s.writeSmartMoneyWatchOpenAlertConfigEnvelope(w, r, user.ID, cfg)
}

func (s *Server) handlePostSmartMoneyWatchOpenAlertConfig(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)

	var req smartMoneyWatchOpenAlertUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, strings.TrimSpace(req.InitData))
	if !ok {
		return
	}

	updates := make(map[string]any)
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.BarkEnabled != nil {
		updates["bark_enabled"] = *req.BarkEnabled
	}
	if req.SoundEnabled != nil {
		updates["sound_enabled"] = *req.SoundEnabled
	}

	repo := smwoa.NewRepository()
	cfg, err := repo.UpdateConfig(r.Context(), user.ID, normalizeSmartMoneyGoldenDogChain(req.Chain), updates)
	if err != nil {
		http.Error(w, "failed to save config", http.StatusInternalServerError)
		return
	}
	s.writeSmartMoneyWatchOpenAlertConfigEnvelope(w, r, user.ID, cfg)
}

func (s *Server) writeSmartMoneyWatchOpenAlertConfigEnvelope(w http.ResponseWriter, r *http.Request, userID uint, cfg *models.SmartMoneyWatchOpenAlertConfig) {
	barkStatus, err := smgd.ResolveUserBarkStatus(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to load bark status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, smartMoneyWatchOpenAlertConfigEnvelope{
		OK:             true,
		Config:         cfg,
		BarkEnabled:    barkStatus.Enabled,
		BarkConfigured: barkStatus.Configured,
		BarkReady:      barkStatus.Ready,
	})
}

func (s *Server) handleSmartMoneyWatchOpenAlertTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)

	var req smartMoneyWatchOpenAlertTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, strings.TrimSpace(req.InitData))
	if !ok {
		return
	}

	barkStatus, err := smgd.ResolveUserBarkStatus(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to load bark status", http.StatusInternalServerError)
		return
	}
	if !barkStatus.Ready {
		http.Error(w, "bark not ready", http.StatusBadRequest)
		return
	}

	chain := strings.ToUpper(normalizeSmartMoneyGoldenDogChain(req.Chain))
	title := "Watch Open Alert Test"
	body := "Watched smart money open alert bark test | chain: " + chain
	if err := notify.SendBarkWithConfig(title, body, barkStatus.Config); err != nil {
		http.Error(w, "failed to send bark test", http.StatusBadGateway)
		return
	}

	writeJSON(w, http.StatusOK, smartMoneyWatchOpenAlertMessageEnvelope{
		OK:      true,
		Message: "Test notification sent",
	})
}

func normalizeSmartMoneyWatchWalletInput(values []string) ([]string, bool) {
	if len(values) == 0 {
		return nil, false
	}
	uniq := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		address := strings.TrimSpace(value)
		if !isValidAddress(address) {
			return nil, true
		}
		address = strings.ToLower(address)
		if _, ok := uniq[address]; ok {
			continue
		}
		uniq[address] = struct{}{}
		out = append(out, address)
	}
	return out, false
}

func smartMoneyWatchWalletChainID(chain string) int {
	switch normalizeSmartMoneyGoldenDogChain(chain) {
	case "base":
		return 8453
	default:
		return 56
	}
}
