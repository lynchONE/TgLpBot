package web_server

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
	sm "TgLpBot/service/smart_money"
	smgd "TgLpBot/service/smart_money_golden_dog"
	smwoa "TgLpBot/service/smart_money_watch_open_alert"
	"encoding/json"
	"net/http"
	"strconv"
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

type smartMoneyWatchActivityItem struct {
	models.SmartMoneyLPEvent
	TradingPair          string  `json:"trading_pair"`
	DisplayTokenAddress  string  `json:"display_token_address,omitempty"`
	DisplayTokenSymbol   string  `json:"display_token_symbol,omitempty"`
	DisplayTokenLogoURL  string  `json:"display_token_logo_url,omitempty"`
	WalletLabel          *string `json:"wallet_label,omitempty"`
	WalletAvatarURL      *string `json:"wallet_avatar_url,omitempty"`
	WalletSource         string  `json:"wallet_source,omitempty"`
	WalletSourceContract string  `json:"wallet_source_contract,omitempty"`
	WalletColor          string  `json:"wallet_color"`
	ExplorerURL          string  `json:"explorer_url,omitempty"`
	FeeDynamic           bool    `json:"fee_dynamic,omitempty"`
}

type smartMoneyWatchActivityEnvelope struct {
	OK      bool                          `json:"ok"`
	Chain   string                        `json:"chain"`
	Wallet  string                        `json:"wallet,omitempty"`
	Page    int                           `json:"page"`
	Size    int                           `json:"size"`
	Total   int64                         `json:"total"`
	List    []smartMoneyWatchActivityItem `json:"list"`
	Wallets []string                      `json:"wallets"`
	Items   []smartMoneyWatchWalletItem   `json:"items"`
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

func (s *Server) handleSmartMoneyWatchActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, _, ok := authenticateSmartMoneyGoldenDogUser(w, initDataFromQuery(r))
	if !ok {
		return
	}

	query := r.URL.Query()
	chain := normalizeSmartMoneyGoldenDogChain(query.Get("chain"))
	chainID := smartMoneyWatchWalletChainID(chain)
	page, _ := strconv.Atoi(query.Get("page"))
	size, _ := strconv.Atoi(query.Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}

	wallet := strings.ToLower(strings.TrimSpace(query.Get("wallet")))
	if wallet != "" && !isValidAddress(wallet) {
		jsonError(w, "invalid wallet", http.StatusBadRequest)
		return
	}

	watchRepo := smwoa.NewRepository()
	watchRows, err := watchRepo.ListWatchWallets(r.Context(), user.ID, chain)
	if err != nil {
		jsonError(w, "failed to load watch wallets", http.StatusInternalServerError)
		return
	}

	walletItems, wallets := s.buildSmartMoneyWatchWalletItems(r, watchRows, chainID)
	if len(watchRows) == 0 {
		writeJSON(w, http.StatusOK, smartMoneyWatchActivityEnvelope{
			OK:      true,
			Chain:   chain,
			Wallet:  wallet,
			Page:    page,
			Size:    size,
			Total:   0,
			List:    []smartMoneyWatchActivityItem{},
			Wallets: wallets,
			Items:   walletItems,
		})
		return
	}

	repo := sm.NewRepository()
	events, total, err := repo.ListWatchLPEvents(r.Context(), sm.WatchActivityQuery{
		UserID:        user.ID,
		ChainID:       chainID,
		WalletAddress: wallet,
		Page:          page,
		Size:          size,
	})
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	list, err := s.buildSmartMoneyWatchActivityItems(r, events)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, smartMoneyWatchActivityEnvelope{
		OK:      true,
		Chain:   chain,
		Wallet:  wallet,
		Page:    page,
		Size:    size,
		Total:   total,
		List:    list,
		Wallets: wallets,
		Items:   walletItems,
	})
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

	items, wallets := s.buildSmartMoneyWatchWalletItems(r, rows, smartMoneyWatchWalletChainID(chain))

	writeJSON(w, http.StatusOK, smartMoneyWatchWalletsEnvelope{
		OK:      true,
		Chain:   chain,
		Count:   len(wallets),
		Wallets: wallets,
		Items:   items,
	})
}

func (s *Server) buildSmartMoneyWatchWalletItems(r *http.Request, rows []models.SmartMoneyWatchWallet, chainID int) ([]smartMoneyWatchWalletItem, []string) {
	items := make([]smartMoneyWatchWalletItem, 0, len(rows))
	wallets := make([]string, 0, len(rows))
	smRepo := sm.NewRepository()

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

	return items, wallets
}

func (s *Server) buildSmartMoneyWatchActivityItems(r *http.Request, events []models.SmartMoneyLPEvent) ([]smartMoneyWatchActivityItem, error) {
	list := make([]smartMoneyWatchActivityItem, 0, len(events))
	if len(events) == 0 {
		return list, nil
	}

	addressesByChain := make(map[string][]string)
	walletCache := make(map[string]*models.MonitoredWallet)
	smRepo := sm.NewRepository()
	for _, event := range events {
		displayAddress, _ := smartMoneyPickDisplayToken(
			event.Token0Address,
			event.Token1Address,
			event.Token0Symbol,
			event.Token1Symbol,
		)
		if displayAddress != "" {
			chain := smartMoneyChainSlug(event.ChainID)
			addressesByChain[chain] = append(addressesByChain[chain], displayAddress)
		}
	}
	metaByChain := s.loadSmartMoneyTokenMetadataByChain(r.Context(), addressesByChain)

	for _, event := range events {
		item := smartMoneyWatchActivityItem{
			SmartMoneyLPEvent: event,
			TradingPair:       buildSmartMoneyTradingPair(event.Token0Symbol, event.Token1Symbol),
			WalletColor:       sm.WalletColor(event.WalletAddress),
			ExplorerURL:       smartMoneyExplorerTxURL(smartMoneyChainSlug(event.ChainID), event.TxHash),
			FeeDynamic:        sm.IsDynamicFeeTier(event.Protocol, event.FeeTier),
		}
		item.DisplayTokenAddress, item.DisplayTokenSymbol = smartMoneyPickDisplayToken(
			event.Token0Address,
			event.Token1Address,
			event.Token0Symbol,
			event.Token1Symbol,
		)
		applySmartMoneyDisplayToken(
			smartMoneyChainSlug(event.ChainID),
			&item.DisplayTokenAddress,
			&item.DisplayTokenSymbol,
			&item.DisplayTokenLogoURL,
			metaByChain,
		)

		walletCacheKey := strconv.Itoa(event.ChainID) + "|" + strings.ToLower(strings.TrimSpace(event.WalletAddress))
		walletRow, ok := walletCache[walletCacheKey]
		if !ok {
			var walletErr error
			walletRow, walletErr = smRepo.GetMonitoredWalletByAddress(r.Context(), event.WalletAddress, event.ChainID)
			if walletErr != nil {
				return nil, walletErr
			}
			walletCache[walletCacheKey] = walletRow
		}
		if walletRow != nil {
			item.WalletLabel = walletRow.Label
			item.WalletAvatarURL = walletRow.AvatarURL
			item.WalletSource = smartMoneyWalletSourceValue(walletRow)
			item.WalletSourceContract = smartMoneyWalletSourceContractValue(walletRow)
		}

		list = append(list, item)
	}

	return list, nil
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

func smartMoneyExplorerTxURL(chain string, txHash string) string {
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return ""
	}
	if url := config.ExplorerTxURL(chain, txHash); strings.TrimSpace(url) != "" {
		return url
	}
	switch config.NormalizeChain(chain) {
	case "base":
		return "https://basescan.org/tx/" + txHash
	default:
		return "https://bscscan.com/tx/" + txHash
	}
}
