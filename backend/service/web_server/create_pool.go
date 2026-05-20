package web_server

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/liquidity"
	poolSvc "TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/txexec"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	createPoolProtocolUniV3 = "univ3"
	createPoolProtocolUniV4 = "univ4"
	createPoolProtocolPcsV3 = "pcsv3"

	createPoolModeCreateOnly    = "create_only"
	createPoolModeCreateAndSeed = "create_and_seed"
	createPoolRangeModeFull     = "full_range"
	createPoolRangeModeCustom   = "custom_range"
	createPoolAmountModeDual    = "dual_exact"
	createPoolAmountModeSingle  = "single_auto_swap"
)

type createPoolRequest struct {
	InitData      string   `json:"initData"`
	WalletID      uint     `json:"wallet_id,omitempty"`
	Chain         string   `json:"chain"`
	Protocol      string   `json:"protocol"`
	TokenAAddress string   `json:"token_a_address"`
	TokenBAddress string   `json:"token_b_address"`
	FeeTier       uint64   `json:"fee_tier"`
	TickSpacing   int      `json:"tick_spacing,omitempty"`
	InitialPrice  string   `json:"initial_price,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	RangeMode     string   `json:"range_mode,omitempty"`
	AmountMode    string   `json:"amount_mode,omitempty"`
	MinPrice      string   `json:"min_price,omitempty"`
	MaxPrice      string   `json:"max_price,omitempty"`
	AmountA       string   `json:"amount_a,omitempty"`
	AmountB       string   `json:"amount_b,omitempty"`
	Slippage      *float64 `json:"slippage_tolerance,omitempty"`
}

type createPoolTokenResponse struct {
	Address          string `json:"address"`
	Symbol           string `json:"symbol"`
	Decimals         int    `json:"decimals"`
	WalletBalance    string `json:"wallet_balance,omitempty"`
	WalletBalanceRaw string `json:"wallet_balance_raw,omitempty"`
}

type createPoolPreviewResponse struct {
	OK                       bool                    `json:"ok"`
	ReadyToExecute           bool                    `json:"ready_to_execute"`
	Chain                    string                  `json:"chain"`
	Protocol                 string                  `json:"protocol"`
	Mode                     string                  `json:"mode"`
	RangeMode                string                  `json:"range_mode"`
	AmountMode               string                  `json:"amount_mode,omitempty"`
	WalletAddress            string                  `json:"wallet_address"`
	TokenA                   createPoolTokenResponse `json:"token_a"`
	TokenB                   createPoolTokenResponse `json:"token_b"`
	Token0                   createPoolTokenResponse `json:"token0"`
	Token1                   createPoolTokenResponse `json:"token1"`
	FeeTier                  uint64                  `json:"fee_tier"`
	TickSpacing              int                     `json:"tick_spacing"`
	InitialPrice             string                  `json:"initial_price,omitempty"`
	InitialPriceSource       string                  `json:"initial_price_source,omitempty"`
	SuggestedInitialPrice    string                  `json:"suggested_initial_price,omitempty"`
	CanonicalPriceToken1Per0 string                  `json:"canonical_price_token1_per_token0,omitempty"`
	MinPrice                 string                  `json:"min_price,omitempty"`
	MaxPrice                 string                  `json:"max_price,omitempty"`
	SqrtPriceX96             string                  `json:"sqrt_price_x96,omitempty"`
	TickLower                int                     `json:"tick_lower,omitempty"`
	TickUpper                int                     `json:"tick_upper,omitempty"`
	AmountA                  string                  `json:"amount_a,omitempty"`
	AmountB                  string                  `json:"amount_b,omitempty"`
	Amount0Desired           string                  `json:"amount0_desired,omitempty"`
	Amount1Desired           string                  `json:"amount1_desired,omitempty"`
	MirrorAmountA            string                  `json:"mirror_amount_a,omitempty"`
	MirrorAmountB            string                  `json:"mirror_amount_b,omitempty"`
	MirrorAmountSource       string                  `json:"mirror_amount_source,omitempty"`
	SingleSidedInput         string                  `json:"single_sided_input,omitempty"`
	EstimatedSwapDirection   string                  `json:"estimated_swap_direction,omitempty"`
	EstimatedSwapAmount      string                  `json:"estimated_swap_amount,omitempty"`
	EstimatedSwapAmountRaw   string                  `json:"estimated_swap_amount_raw,omitempty"`
	EstimatedSwapTokenIn     string                  `json:"estimated_swap_token_in,omitempty"`
	EstimatedSwapTokenOut    string                  `json:"estimated_swap_token_out,omitempty"`
	EstimatedLiquidity       string                  `json:"estimated_liquidity,omitempty"`
	PoolExists               bool                    `json:"pool_exists"`
	ExistingPoolAddress      string                  `json:"existing_pool_address,omitempty"`
	ExistingPoolID           string                  `json:"existing_pool_id,omitempty"`
	PredictedPoolID          string                  `json:"predicted_pool_id,omitempty"`
	Warnings                 []string                `json:"warnings,omitempty"`
}

type createPoolExecuteResponse struct {
	OK            bool     `json:"ok"`
	Status        string   `json:"status"`
	Chain         string   `json:"chain"`
	Protocol      string   `json:"protocol"`
	Mode          string   `json:"mode"`
	WalletAddress string   `json:"wallet_address"`
	PoolAddress   string   `json:"pool_address,omitempty"`
	PoolID        string   `json:"pool_id,omitempty"`
	TokenID       string   `json:"token_id,omitempty"`
	Liquidity     string   `json:"liquidity,omitempty"`
	TxHash        string   `json:"tx_hash"`
	TxHashes      []string `json:"tx_hashes,omitempty"`
	ExplorerURLs  []string `json:"explorer_urls,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

type createPoolContext struct {
	user    *models.User
	wallet  *models.Wallet
	exec    chainexec.EVMExecutor
	chain   string
	cc      config.ChainConfig
	client  *ethclient.Client
	chainID *big.Int
}

type createPoolTokenMeta struct {
	Address          common.Address
	AddressHex       string
	Symbol           string
	Decimals         int
	WalletBalance    *big.Int
	WalletBalanceStr string
}

type createPoolPlan struct {
	ctx                      *createPoolContext
	protocol                 string
	mode                     string
	rangeMode                string
	amountMode               string
	slippagePct              float64
	tokenA                   createPoolTokenMeta
	tokenB                   createPoolTokenMeta
	token0                   createPoolTokenMeta
	token1                   createPoolTokenMeta
	tokenAIsToken0           bool
	feeTier                  uint64
	tickSpacing              int
	initialPriceAB           *big.Float
	initialPriceABText       string
	initialPriceSource       string
	canonicalPriceToken1Per0 *big.Float
	canonicalPriceText       string
	currentTick              int
	sqrtPriceX96             *big.Int
	tickLower                int
	tickUpper                int
	minPriceABText           string
	maxPriceABText           string
	amountAInput             string
	amountBInput             string
	amountAUnits             *big.Int
	amountBUnits             *big.Int
	amount0Desired           *big.Int
	amount1Desired           *big.Int
	mirrorAmountAUnits       *big.Int
	mirrorAmountBUnits       *big.Int
	mirrorAmountA            string
	mirrorAmountB            string
	mirrorAmountSource       string
	singleSidedInput         string
	singleInputToken         common.Address
	singleInputAmount        *big.Int
	singleInputAmountText    string
	estimatedSwapDirection   string
	estimatedSwapAmount      *big.Int
	estimatedSwapAmountText  string
	estimatedSwapTokenIn     common.Address
	estimatedSwapTokenOut    common.Address
	estimatedLiquidity       *big.Int
	poolExists               bool
	existingPoolAddress      common.Address
	predictedPoolID          common.Hash
	positionManager          common.Address
	factory                  common.Address
	hooks                    common.Address
	warnings                 []string
}

func normalizeCreatePoolProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "univ3", "uniswapv3", "uniswap_v3":
		return createPoolProtocolUniV3
	case "univ4", "uniswapv4", "uniswap_v4":
		return createPoolProtocolUniV4
	case "pcsv3", "pancakev3", "pancakeswap_v3", "pancake_v3":
		return createPoolProtocolPcsV3
	default:
		return ""
	}
}

func normalizeCreatePoolMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", createPoolModeCreateAndSeed:
		return createPoolModeCreateAndSeed
	case createPoolModeCreateOnly:
		return createPoolModeCreateOnly
	default:
		return ""
	}
}

func normalizeCreatePoolRangeMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", createPoolRangeModeFull:
		return createPoolRangeModeFull
	case createPoolRangeModeCustom:
		return createPoolRangeModeCustom
	default:
		return ""
	}
}

func normalizeCreatePoolAmountMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", createPoolAmountModeDual:
		return createPoolAmountModeDual
	case createPoolAmountModeSingle:
		return createPoolAmountModeSingle
	default:
		return ""
	}
}

func formatCreatePoolDecimal(v *big.Float, scale int) string {
	if v == nil {
		return ""
	}
	if scale < 0 {
		scale = 0
	}
	s := v.Text('f', scale)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

func explorerURLsForTxs(cc config.ChainConfig, hashes []string) []string {
	tpl := strings.TrimSpace(cc.ExplorerTxURLTemplate)
	if tpl == "" || len(hashes) == 0 {
		return nil
	}
	out := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash == "" {
			continue
		}
		out = append(out, fmt.Sprintf(tpl, hash))
	}
	return out
}

func findPriceValue(prices map[string]float64, token common.Address) float64 {
	if len(prices) == 0 || token == (common.Address{}) {
		return 0
	}
	keys := []string{
		strings.ToLower(strings.TrimSpace(token.Hex())),
		strings.TrimSpace(token.Hex()),
	}
	for _, key := range keys {
		if v := prices[key]; v > 0 {
			return v
		}
	}
	return 0
}

func shortSymbolFallback(addr common.Address) string {
	hex := addr.Hex()
	if len(hex) <= 10 {
		return hex
	}
	return fmt.Sprintf("%s..%s", hex[:6], hex[len(hex)-4:])
}

func balanceToDecimalString(amount *big.Int, decimals int) string {
	if amount == nil {
		return ""
	}
	value := amountToFloat(amount.String(), decimals)
	if value == 0 {
		return "0"
	}
	if value >= 1000 {
		return fmt.Sprintf("%.2f", value)
	}
	if value >= 1 {
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.10f", value), "0"), ".")
}

func createPoolTokenResponseFromMeta(meta createPoolTokenMeta) createPoolTokenResponse {
	return createPoolTokenResponse{
		Address:          meta.AddressHex,
		Symbol:           meta.Symbol,
		Decimals:         meta.Decimals,
		WalletBalance:    meta.WalletBalanceStr,
		WalletBalanceRaw: amountString(meta.WalletBalance),
	}
}

func amountString(v *big.Int) string {
	if v == nil {
		return ""
	}
	return v.String()
}

func containsCreatePoolNotInitialized(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "not initialized") || strings.Contains(msg, "sqrtpricex96=0")
}

func parseCreatePoolMintedTokenID(receipt *types.Receipt, nft common.Address, to common.Address) *big.Int {
	if receipt == nil || nft == (common.Address{}) || to == (common.Address{}) {
		return nil
	}
	transferEventID := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != nft || len(lg.Topics) < 4 || lg.Topics[0] != transferEventID {
			continue
		}
		fromAddr := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
		toAddr := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
		if fromAddr != (common.Address{}) || toAddr != to {
			continue
		}
		tokenID := new(big.Int).SetBytes(lg.Topics[3].Bytes())
		if tokenID.Sign() > 0 {
			return tokenID
		}
	}
	return nil
}

func parseCreatePoolZapInV4(receipt *types.Receipt, zapAddr common.Address) (*big.Int, *big.Int, bool) {
	if receipt == nil || zapAddr == (common.Address{}) {
		return nil, nil, false
	}
	parsed, err := abi.JSON(strings.NewReader(blockchain.ZapSimpleABI))
	if err != nil {
		return nil, nil, false
	}
	ev, ok := parsed.Events["ZapInV4"]
	if !ok {
		return nil, nil, false
	}
	for _, lg := range receipt.Logs {
		if lg == nil || lg.Address != zapAddr || len(lg.Topics) < 4 || lg.Topics[0] != ev.ID {
			continue
		}
		tokenID := new(big.Int).SetBytes(lg.Topics[3].Bytes())
		out, err := parsed.Unpack("ZapInV4", lg.Data)
		if err != nil || len(out) < 3 {
			return tokenID, big.NewInt(0), true
		}
		liq, _ := out[2].(*big.Int)
		if liq == nil {
			liq = big.NewInt(0)
		}
		return tokenID, liq, true
	}
	return nil, nil, false
}

func bytesCompare(a []byte, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

func (s *Server) resolveCreatePoolContext(req *createPoolRequest) (*createPoolContext, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}

	user, status, msg := authenticateTelegramWebAppUser(strings.TrimSpace(req.InitData))
	if status != 0 {
		return nil, fmt.Errorf("%s", msg)
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		return nil, fmt.Errorf("%s", msg)
	}
	if status != 0 {
		return nil, fmt.Errorf("%s", msg)
	}
	if status, msg := requireModulePermission(check, models.AccessModuleCreatePool); status != 0 {
		return nil, fmt.Errorf("%s", msg)
	}

	chain := config.NormalizeChain(strings.TrimSpace(req.Chain))
	if chain == "" {
		chain = "bsc"
	}
	if chain != "bsc" {
		return nil, fmt.Errorf("pool creation currently supports bsc only")
	}

	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		return nil, err
	}
	cc := exec.Config()
	if strings.TrimSpace(cc.Chain) == "" {
		return nil, fmt.Errorf("invalid chain config")
	}

	walletSvc := wallet.NewWalletService()
	wallets, err := walletSvc.GetUserWallets(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load wallets: %w", err)
	}
	if len(wallets) == 0 {
		return nil, fmt.Errorf("no wallet found")
	}

	selectedWallet := &wallets[0]
	for i := range wallets {
		if wallets[i].IsDefault {
			selectedWallet = &wallets[i]
			break
		}
	}
	if req.WalletID > 0 {
		selectedWallet, err = walletSvc.GetWalletByID(user.ID, req.WalletID)
		if err != nil || selectedWallet == nil {
			return nil, fmt.Errorf("invalid wallet")
		}
	}

	return &createPoolContext{
		user:    user,
		wallet:  selectedWallet,
		exec:    exec,
		chain:   chain,
		cc:      cc,
		client:  exec.Client(),
		chainID: exec.ChainID(),
	}, nil
}

func (s *Server) loadCreatePoolTokenMeta(ctx *createPoolContext, raw string) (createPoolTokenMeta, error) {
	var meta createPoolTokenMeta
	raw = strings.TrimSpace(raw)
	if !common.IsHexAddress(raw) {
		return meta, fmt.Errorf("invalid token address: %s", raw)
	}
	addr := common.HexToAddress(raw)
	decimals, err := blockchain.GetTokenDecimalsWithClient(ctx.client, addr)
	if err != nil {
		return meta, fmt.Errorf("read token decimals failed for %s: %w", addr.Hex(), err)
	}
	symbol, err := blockchain.GetTokenSymbolWithClient(ctx.client, addr)
	if err != nil || strings.TrimSpace(symbol) == "" {
		symbol = shortSymbolFallback(addr)
	}
	balance, _ := blockchain.GetTokenBalanceWithClient(ctx.client, addr, common.HexToAddress(ctx.wallet.Address))
	if balance == nil {
		balance = big.NewInt(0)
	}

	meta = createPoolTokenMeta{
		Address:          addr,
		AddressHex:       addr.Hex(),
		Symbol:           strings.TrimSpace(symbol),
		Decimals:         int(decimals),
		WalletBalance:    balance,
		WalletBalanceStr: balanceToDecimalString(balance, int(decimals)),
	}
	return meta, nil
}

func (s *Server) resolveCreatePoolPrice(plan *createPoolPlan, raw string) (*big.Float, string, string, []string, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		value, err := poolSvc.ParseDecimalToFloat(raw)
		if err != nil {
			return nil, "", "", nil, fmt.Errorf("invalid initial_price: %w", err)
		}
		return value, formatCreatePoolDecimal(value, 12), "manual", nil, nil
	}

	if s.TokenPrice == nil {
		s.TokenPrice = pricing.NewTokenPriceService()
	}
	prices, err := s.TokenPrice.GetUSDPrices(plan.ctx.chain, []string{
		plan.tokenA.AddressHex,
		plan.tokenB.AddressHex,
	})
	if err != nil {
		return nil, "", "", []string{fmt.Sprintf("自动推导初始价格失败: %v", err)}, nil
	}
	priceA := findPriceValue(prices, plan.tokenA.Address)
	priceB := findPriceValue(prices, plan.tokenB.Address)
	if priceA <= 0 || priceB <= 0 {
		return nil, "", "", []string{"无法自动推导初始价格，请手动输入 `1 TokenA = X TokenB`"}, nil
	}

	value := new(big.Float).SetPrec(256).Quo(
		new(big.Float).SetPrec(256).SetFloat64(priceA),
		new(big.Float).SetPrec(256).SetFloat64(priceB),
	)
	text := formatCreatePoolDecimal(value, 12)
	return value, text, "usd_ratio", nil, nil
}

func (s *Server) prepareCreatePoolPlanLegacy(req *createPoolRequest, requireAmounts bool) (*createPoolPlan, *createPoolPreviewResponse, error) {
	ctx, err := s.resolveCreatePoolContext(req)
	if err != nil {
		return nil, nil, err
	}
	if ctx.client == nil || ctx.chainID == nil || ctx.chainID.Sign() <= 0 {
		return nil, nil, fmt.Errorf("blockchain client not initialized")
	}

	protocol := normalizeCreatePoolProtocol(req.Protocol)
	if protocol == "" {
		return nil, nil, fmt.Errorf("unsupported protocol")
	}
	mode := normalizeCreatePoolMode(req.Mode)
	if mode == "" {
		return nil, nil, fmt.Errorf("unsupported mode")
	}
	rangeMode := normalizeCreatePoolRangeMode(req.RangeMode)
	if rangeMode == "" {
		return nil, nil, fmt.Errorf("unsupported range mode")
	}
	slippagePct := 0.5
	if req.Slippage != nil {
		if *req.Slippage < 0 || *req.Slippage > 100 {
			return nil, nil, fmt.Errorf("invalid slippage_tolerance")
		}
		slippagePct = *req.Slippage
	}

	tokenA, err := s.loadCreatePoolTokenMeta(ctx, req.TokenAAddress)
	if err != nil {
		return nil, nil, err
	}
	tokenB, err := s.loadCreatePoolTokenMeta(ctx, req.TokenBAddress)
	if err != nil {
		return nil, nil, err
	}
	if tokenA.Address == tokenB.Address {
		return nil, nil, fmt.Errorf("token_a_address and token_b_address must be different")
	}

	token0 := tokenA
	token1 := tokenB
	tokenAIsToken0 := true
	if bytesCompare(tokenA.Address.Bytes(), tokenB.Address.Bytes()) > 0 {
		token0 = tokenB
		token1 = tokenA
		tokenAIsToken0 = false
	}

	plan := &createPoolPlan{
		ctx:            ctx,
		protocol:       protocol,
		mode:           mode,
		rangeMode:      rangeMode,
		slippagePct:    slippagePct,
		tokenA:         tokenA,
		tokenB:         tokenB,
		token0:         token0,
		token1:         token1,
		tokenAIsToken0: tokenAIsToken0,
		feeTier:        req.FeeTier,
		amountAInput:   strings.TrimSpace(req.AmountA),
		amountBInput:   strings.TrimSpace(req.AmountB),
		hooks:          common.Address{},
	}

	if plan.feeTier == 0 {
		return nil, nil, fmt.Errorf("missing fee_tier")
	}

	switch plan.protocol {
	case createPoolProtocolUniV3, createPoolProtocolPcsV3:
		var deploymentName string
		switch plan.protocol {
		case createPoolProtocolUniV3:
			deploymentName = "uniswap v3"
		default:
			deploymentName = "pancakeswap v3"
		}
		for _, dep := range ctx.cc.V3Deployments {
			if strings.EqualFold(strings.TrimSpace(dep.Name), deploymentName) {
				if common.IsHexAddress(dep.FactoryAddress) {
					plan.factory = common.HexToAddress(dep.FactoryAddress)
				}
				if common.IsHexAddress(dep.PositionManagerAddress) {
					plan.positionManager = common.HexToAddress(dep.PositionManagerAddress)
				}
				break
			}
		}
		if plan.factory == (common.Address{}) || plan.positionManager == (common.Address{}) {
			return nil, nil, fmt.Errorf("target v3 deployment is not configured")
		}
		tickSpacing, err := blockchain.GetV3FeeAmountTickSpacingWithClient(ctx.client, plan.factory, plan.feeTier)
		if err != nil || tickSpacing <= 0 {
			return nil, nil, fmt.Errorf("fee tier %d is not supported by target protocol", plan.feeTier)
		}
		plan.tickSpacing = tickSpacing
		poolAddr, err := blockchain.GetV3PoolFromFactoryWithClient(ctx.client, plan.factory, plan.token0.Address, plan.token1.Address, plan.feeTier)
		if err == nil && poolAddr != (common.Address{}) {
			plan.poolExists = true
			plan.existingPoolAddress = poolAddr
		}

	case createPoolProtocolUniV4:
		if !common.IsHexAddress(ctx.cc.UniswapV4PositionManagerAddress) {
			return nil, nil, fmt.Errorf("UNISWAP_V4_POSITION_MANAGER_ADDRESS not configured")
		}
		if !common.IsHexAddress(ctx.cc.UniswapV4PoolManagerAddress) {
			return nil, nil, fmt.Errorf("UNISWAP_V4_POOL_MANAGER_ADDRESS not configured")
		}
		if !common.IsHexAddress(ctx.cc.UniswapV4StateViewAddress) {
			return nil, nil, fmt.Errorf("UNISWAP_V4_STATE_VIEW_ADDRESS not configured")
		}
		switch plan.feeTier {
		case 100, 500, 3000, 10000:
		default:
			return nil, nil, fmt.Errorf("fee tier %d is not supported by uniswap v4 static fees", plan.feeTier)
		}
		tickSpacing, err := poolSvc.StandardTickSpacingFromFee(plan.feeTier)
		if err != nil {
			return nil, nil, err
		}
		plan.tickSpacing = tickSpacing
		plan.positionManager = common.HexToAddress(ctx.cc.UniswapV4PositionManagerAddress)
		plan.predictedPoolID, err = blockchain.ComputeUniswapV4PoolID(
			plan.token0.Address,
			plan.token1.Address,
			plan.feeTier,
			plan.tickSpacing,
			plan.hooks,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("compute predicted pool id failed: %w", err)
		}
		stateView := common.HexToAddress(ctx.cc.UniswapV4StateViewAddress)
		poolManager := common.HexToAddress(ctx.cc.UniswapV4PoolManagerAddress)
		if sqrt, _, err := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateView, poolManager, plan.predictedPoolID.Hex()); err == nil && sqrt != nil && sqrt.Sign() > 0 {
			plan.poolExists = true
		} else if err != nil && !containsCreatePoolNotInitialized(err) {
			plan.warnings = append(plan.warnings, fmt.Sprintf("V4 池子存在性检查未能确认，将在执行前再次校验: %v", err))
		}
	default:
		return nil, nil, fmt.Errorf("unsupported protocol")
	}

	initialPrice, initialPriceText, priceSource, priceWarnings, err := s.resolveCreatePoolPrice(plan, req.InitialPrice)
	if err != nil {
		return nil, nil, err
	}
	plan.warnings = append(plan.warnings, priceWarnings...)
	plan.initialPriceAB = initialPrice
	plan.initialPriceABText = initialPriceText
	plan.initialPriceSource = priceSource

	if plan.initialPriceAB != nil {
		canonicalPrice, err := poolSvc.HumanPriceFromBaseQuote(plan.initialPriceAB, plan.tokenAIsToken0)
		if err != nil {
			return nil, nil, err
		}
		plan.canonicalPriceToken1Per0 = canonicalPrice
		plan.canonicalPriceText = formatCreatePoolDecimal(canonicalPrice, 12)
		plan.sqrtPriceX96, err = poolSvc.SqrtPriceX96FromHumanPrice(canonicalPrice, plan.token0.Decimals, plan.token1.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("compute sqrtPriceX96 failed: %w", err)
		}
	}

	plan.tickLower, plan.tickUpper, err = poolSvc.FullRangeTicks(plan.tickSpacing)
	if err != nil {
		return nil, nil, err
	}

	if plan.amountAInput != "" {
		plan.amountAUnits, err = poolSvc.DecimalToUnits(plan.amountAInput, plan.tokenA.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid amount_a: %w", err)
		}
	}
	if plan.amountBInput != "" {
		plan.amountBUnits, err = poolSvc.DecimalToUnits(plan.amountBInput, plan.tokenB.Decimals)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid amount_b: %w", err)
		}
	}
	if plan.amountAUnits != nil || plan.amountBUnits != nil {
		if plan.amountAUnits == nil || plan.amountBUnits == nil {
			plan.warnings = append(plan.warnings, "建池+首注需要同时提供 Token A / Token B 数量")
		} else {
			if plan.tokenAIsToken0 {
				plan.amount0Desired = new(big.Int).Set(plan.amountAUnits)
				plan.amount1Desired = new(big.Int).Set(plan.amountBUnits)
			} else {
				plan.amount0Desired = new(big.Int).Set(plan.amountBUnits)
				plan.amount1Desired = new(big.Int).Set(plan.amountAUnits)
			}
			if plan.sqrtPriceX96 != nil {
				sqrtLower, err := poolSvc.SqrtRatioAtTick(int32(plan.tickLower))
				if err != nil {
					return nil, nil, err
				}
				sqrtUpper, err := poolSvc.SqrtRatioAtTick(int32(plan.tickUpper))
				if err != nil {
					return nil, nil, err
				}
				plan.estimatedLiquidity = poolSvc.LiquidityForAmounts(
					plan.sqrtPriceX96,
					sqrtLower,
					sqrtUpper,
					plan.amount0Desired,
					plan.amount1Desired,
				)
			}
			if plan.tokenA.WalletBalance != nil && plan.tokenA.WalletBalance.Cmp(plan.amountAUnits) < 0 {
				plan.warnings = append(plan.warnings, fmt.Sprintf("Token A 余额不足: 当前 %s %s", plan.tokenA.WalletBalanceStr, plan.tokenA.Symbol))
			}
			if plan.tokenB.WalletBalance != nil && plan.tokenB.WalletBalance.Cmp(plan.amountBUnits) < 0 {
				plan.warnings = append(plan.warnings, fmt.Sprintf("Token B 余额不足: 当前 %s %s", plan.tokenB.WalletBalanceStr, plan.tokenB.Symbol))
			}
		}
	} else if plan.mode == createPoolModeCreateAndSeed {
		plan.warnings = append(plan.warnings, "建池+首注需要填写 Token A / Token B 数量")
	}

	if requireAmounts && plan.mode == createPoolModeCreateAndSeed && (plan.amount0Desired == nil || plan.amount1Desired == nil) {
		return nil, nil, fmt.Errorf("missing amount_a or amount_b")
	}
	if requireAmounts && plan.initialPriceAB == nil {
		return nil, nil, fmt.Errorf("missing initial_price and failed to derive a suggested price")
	}
	if requireAmounts && plan.poolExists {
		switch plan.protocol {
		case createPoolProtocolUniV4:
			return nil, nil, fmt.Errorf("target v4 pool already exists")
		default:
			return nil, nil, fmt.Errorf("target v3 pool already exists at %s", plan.existingPoolAddress.Hex())
		}
	}

	readyToExecute := plan.initialPriceAB != nil && !plan.poolExists
	if plan.mode == createPoolModeCreateAndSeed {
		readyToExecute = readyToExecute && plan.amount0Desired != nil && plan.amount1Desired != nil
	}

	resp := &createPoolPreviewResponse{
		OK:                       true,
		ReadyToExecute:           readyToExecute,
		Chain:                    plan.ctx.chain,
		Protocol:                 plan.protocol,
		Mode:                     plan.mode,
		RangeMode:                plan.rangeMode,
		WalletAddress:            strings.TrimSpace(plan.ctx.wallet.Address),
		TokenA:                   createPoolTokenResponseFromMeta(plan.tokenA),
		TokenB:                   createPoolTokenResponseFromMeta(plan.tokenB),
		Token0:                   createPoolTokenResponseFromMeta(plan.token0),
		Token1:                   createPoolTokenResponseFromMeta(plan.token1),
		FeeTier:                  plan.feeTier,
		TickSpacing:              plan.tickSpacing,
		InitialPrice:             plan.initialPriceABText,
		InitialPriceSource:       plan.initialPriceSource,
		SuggestedInitialPrice:    plan.initialPriceABText,
		CanonicalPriceToken1Per0: plan.canonicalPriceText,
		SqrtPriceX96:             amountString(plan.sqrtPriceX96),
		TickLower:                plan.tickLower,
		TickUpper:                plan.tickUpper,
		AmountA:                  amountString(plan.amountAUnits),
		AmountB:                  amountString(plan.amountBUnits),
		Amount0Desired:           amountString(plan.amount0Desired),
		Amount1Desired:           amountString(plan.amount1Desired),
		EstimatedLiquidity:       amountString(plan.estimatedLiquidity),
		PoolExists:               plan.poolExists,
		Warnings:                 plan.warnings,
	}
	if plan.existingPoolAddress != (common.Address{}) {
		resp.ExistingPoolAddress = plan.existingPoolAddress.Hex()
	}
	if plan.predictedPoolID != (common.Hash{}) {
		resp.PredictedPoolID = plan.predictedPoolID.Hex()
		if plan.poolExists {
			resp.ExistingPoolID = plan.predictedPoolID.Hex()
		}
	}
	return plan, resp, nil
}

func (s *Server) executeCreatePoolPlanLegacy(plan *createPoolPlan) (*createPoolExecuteResponse, error) {
	if plan == nil || plan.ctx == nil {
		return nil, fmt.Errorf("create pool plan is nil")
	}

	walletSvc := wallet.NewWalletService()
	privateKeyHex, err := walletSvc.GetPrivateKey(plan.ctx.wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to load wallet private key: %w", err)
	}
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid wallet private key: %w", err)
	}

	liqSvc := liquidity.NewLiquidityService()
	walletAddr := common.HexToAddress(plan.ctx.wallet.Address)
	var txHashes []string

	switch plan.protocol {
	case createPoolProtocolUniV3, createPoolProtocolPcsV3:
		v3pm, err := blockchain.NewV3PositionManager(plan.positionManager, plan.ctx.client)
		if err != nil {
			return nil, fmt.Errorf("init v3 position manager failed: %w", err)
		}

		if plan.mode == createPoolModeCreateAndSeed {
			if err := liqSvc.ApproveTokenForSpender(plan.ctx.client, plan.ctx.chainID, privateKey, walletAddr, plan.token0.Address, plan.positionManager, plan.amount0Desired, liquidity.TxOptions{}); err != nil {
				return nil, fmt.Errorf("approve token0 failed: %w", err)
			}
			if err := liqSvc.ApproveTokenForSpender(plan.ctx.client, plan.ctx.chainID, privateKey, walletAddr, plan.token1.Address, plan.positionManager, plan.amount1Desired, liquidity.TxOptions{}); err != nil {
				return nil, fmt.Errorf("approve token1 failed: %w", err)
			}
		}

		feeBig := new(big.Int).SetUint64(plan.feeTier)
		initData, err := v3pm.Pack(
			"createAndInitializePoolIfNecessary",
			plan.token0.Address,
			plan.token1.Address,
			feeBig,
			plan.sqrtPriceX96,
		)
		if err != nil {
			return nil, fmt.Errorf("pack v3 init calldata failed: %w", err)
		}

		nonce, err := blockchain.GetNonceWithClient(plan.ctx.client, walletAddr)
		if err != nil {
			return nil, err
		}
		auth, err := liqSvc.BuildAuthForTx(plan.ctx.client, plan.ctx.chainID, privateKey, nonce, big.NewInt(0), liquidity.TxOptions{})
		if err != nil {
			return nil, err
		}

		var tx *types.Transaction
		if plan.mode == createPoolModeCreateOnly {
			tx, err = v3pm.CreateAndInitializePoolIfNecessary(auth, plan.token0.Address, plan.token1.Address, feeBig, plan.sqrtPriceX96)
			if err != nil {
				return nil, fmt.Errorf("create v3 pool failed: %w", err)
			}
		} else {
			mintParams := blockchain.V3MintParams{
				Token0:         plan.token0.Address,
				Token1:         plan.token1.Address,
				Fee:            feeBig,
				TickLower:      big.NewInt(int64(plan.tickLower)),
				TickUpper:      big.NewInt(int64(plan.tickUpper)),
				Amount0Desired: plan.amount0Desired,
				Amount1Desired: plan.amount1Desired,
				Amount0Min:     big.NewInt(0),
				Amount1Min:     big.NewInt(0),
				Recipient:      walletAddr,
				Deadline:       big.NewInt(time.Now().Add(20 * time.Minute).Unix()),
			}
			mintData, err := v3pm.Pack("mint", mintParams)
			if err != nil {
				return nil, fmt.Errorf("pack v3 mint calldata failed: %w", err)
			}
			tx, err = v3pm.Multicall(auth, [][]byte{initData, mintData})
			if err != nil {
				return nil, fmt.Errorf("create+mint v3 pool failed: %w", err)
			}
		}

		txHashes = append(txHashes, tx.Hash().Hex())
		receipt, err := liqSvc.WaitMinedTx(plan.ctx.client, plan.ctx.chainID, tx)
		if err != nil {
			return nil, fmt.Errorf("v3 create pool transaction failed: %w", err)
		}

		poolAddr, err := blockchain.GetV3PoolFromFactoryWithClient(plan.ctx.client, plan.factory, plan.token0.Address, plan.token1.Address, plan.feeTier)
		if err != nil {
			return nil, fmt.Errorf("resolve created v3 pool failed: %w", err)
		}
		resp := &createPoolExecuteResponse{
			OK:            true,
			Status:        "ok",
			Chain:         plan.ctx.chain,
			Protocol:      plan.protocol,
			Mode:          plan.mode,
			WalletAddress: plan.ctx.wallet.Address,
			PoolAddress:   poolAddr.Hex(),
			TxHash:        tx.Hash().Hex(),
			TxHashes:      txHashes,
			ExplorerURLs:  explorerURLsForTxs(plan.ctx.cc, txHashes),
			Warnings:      plan.warnings,
		}
		if plan.mode == createPoolModeCreateAndSeed {
			if tokenID := parseCreatePoolMintedTokenID(receipt, plan.positionManager, walletAddr); tokenID != nil {
				resp.TokenID = tokenID.String()
				if pos, err := v3pm.Positions(&bind.CallOpts{}, tokenID); err == nil && pos != nil && pos.Liquidity != nil {
					resp.Liquidity = pos.Liquidity.String()
				}
			}
		}
		return resp, nil

	case createPoolProtocolUniV4:
		v4pm, err := blockchain.NewV4PositionManager(plan.positionManager, plan.ctx.client)
		if err != nil {
			return nil, fmt.Errorf("init v4 position manager failed: %w", err)
		}

		nonce, err := blockchain.GetNonceWithClient(plan.ctx.client, walletAddr)
		if err != nil {
			return nil, err
		}
		auth, err := liqSvc.BuildAuthForTx(plan.ctx.client, plan.ctx.chainID, privateKey, nonce, big.NewInt(0), liquidity.TxOptions{})
		if err != nil {
			return nil, err
		}
		key := blockchain.V4PoolKey{
			Currency0:   plan.token0.Address,
			Currency1:   plan.token1.Address,
			Fee:         new(big.Int).SetUint64(plan.feeTier),
			TickSpacing: big.NewInt(int64(plan.tickSpacing)),
			Hooks:       plan.hooks,
		}
		initTx, err := v4pm.InitializePool(auth, key, plan.sqrtPriceX96)
		if err != nil {
			return nil, fmt.Errorf("initialize v4 pool failed: %w", err)
		}
		txHashes = append(txHashes, initTx.Hash().Hex())
		if _, err := liqSvc.WaitMinedTx(plan.ctx.client, plan.ctx.chainID, initTx); err != nil {
			return nil, fmt.Errorf("v4 initialize pool transaction failed: %w", err)
		}

		resp := &createPoolExecuteResponse{
			OK:            true,
			Status:        "ok",
			Chain:         plan.ctx.chain,
			Protocol:      plan.protocol,
			Mode:          plan.mode,
			WalletAddress: plan.ctx.wallet.Address,
			PoolID:        plan.predictedPoolID.Hex(),
			TxHash:        initTx.Hash().Hex(),
			TxHashes:      txHashes,
			Warnings:      plan.warnings,
		}

		if plan.mode == createPoolModeCreateOnly {
			resp.ExplorerURLs = explorerURLsForTxs(plan.ctx.cc, txHashes)
			return resp, nil
		}

		if !common.IsHexAddress(plan.ctx.cc.ZapV4Address) {
			return nil, fmt.Errorf("ZAP_V4_ADDRESS not configured")
		}
		zapAddr := common.HexToAddress(plan.ctx.cc.ZapV4Address)
		if err := liqSvc.ApproveTokenForSpender(plan.ctx.client, plan.ctx.chainID, privateKey, walletAddr, plan.token0.Address, zapAddr, plan.amount0Desired, liquidity.TxOptions{}); err != nil {
			return nil, fmt.Errorf("approve token0 to zap failed: %w", err)
		}
		if err := liqSvc.ApproveTokenForSpender(plan.ctx.client, plan.ctx.chainID, privateKey, walletAddr, plan.token1.Address, zapAddr, plan.amount1Desired, liquidity.TxOptions{}); err != nil {
			return nil, fmt.Errorf("approve token1 to zap failed: %w", err)
		}

		zap, err := blockchain.NewZapSimple(zapAddr, plan.ctx.client)
		if err != nil {
			return nil, fmt.Errorf("init zap v4 failed: %w", err)
		}
		nonce, err = blockchain.GetNonceWithClient(plan.ctx.client, walletAddr)
		if err != nil {
			return nil, err
		}
		auth, err = liqSvc.BuildAuthForTx(plan.ctx.client, plan.ctx.chainID, privateKey, nonce, big.NewInt(0), liquidity.TxOptions{})
		if err != nil {
			return nil, err
		}
		zapParams := blockchain.ZapInV4ParamsSimple{
			PoolKey: blockchain.PoolKeySimple{
				Currency0:   plan.token0.Address,
				Currency1:   plan.token1.Address,
				Fee:         new(big.Int).SetUint64(plan.feeTier),
				TickSpacing: big.NewInt(int64(plan.tickSpacing)),
				Hooks:       plan.hooks,
			},
			StateView:       common.HexToAddress(plan.ctx.cc.UniswapV4StateViewAddress),
			PositionManager: plan.positionManager,
			TickLower:       big.NewInt(int64(plan.tickLower)),
			TickUpper:       big.NewInt(int64(plan.tickUpper)),
			Recipient:       walletAddr,
			Amount0In:       plan.amount0Desired,
			Amount1In:       plan.amount1Desired,
			SlippageBps:     liquidity.V4PriceMoveToleranceBps(plan.slippagePct),
			Swap: blockchain.SwapParamsSimple{
				Target:        common.Address{},
				ApproveTarget: common.Address{},
				TokenIn:       common.Address{},
				TokenOut:      common.Address{},
				AmountIn:      big.NewInt(0),
				MinAmountOut:  big.NewInt(0),
				CallData:      []byte{},
			},
			SqrtPriceX96: plan.sqrtPriceX96,
		}
		seedTx, err := zap.ZapInV4(auth, zapParams)
		if err != nil {
			return nil, fmt.Errorf("seed v4 pool failed: %w", err)
		}
		txHashes = append(txHashes, seedTx.Hash().Hex())
		seedReceipt, err := liqSvc.WaitMinedTx(plan.ctx.client, plan.ctx.chainID, seedTx)
		if err != nil {
			return nil, fmt.Errorf("v4 seed pool transaction failed: %w", err)
		}

		resp.TxHash = seedTx.Hash().Hex()
		resp.TxHashes = txHashes
		resp.ExplorerURLs = explorerURLsForTxs(plan.ctx.cc, txHashes)
		if tokenID, liq, ok := parseCreatePoolZapInV4(seedReceipt, zapAddr); ok {
			if tokenID != nil {
				resp.TokenID = tokenID.String()
			}
			if liq != nil {
				resp.Liquidity = liq.String()
			}
		} else if tokenID := parseCreatePoolMintedTokenID(seedReceipt, plan.positionManager, walletAddr); tokenID != nil {
			resp.TokenID = tokenID.String()
			poolManagerAddr := common.Address{}
			if common.IsHexAddress(plan.ctx.cc.UniswapV4PoolManagerAddress) {
				poolManagerAddr = common.HexToAddress(plan.ctx.cc.UniswapV4PoolManagerAddress)
			}
			if pos, err := blockchain.GetV4PositionInfo(plan.positionManager, poolManagerAddr, plan.predictedPoolID.Hex(), tokenID); err == nil && pos != nil && pos.Liquidity != nil {
				resp.Liquidity = pos.Liquidity.String()
			}
		}
		return resp, nil

	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}

func (s *Server) handleCreatePoolPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var req createPoolRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	_, resp, err := s.prepareCreatePoolPlan(&req, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreatePoolExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var req createPoolRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	plan, _, err := s.prepareCreatePoolPlan(&req, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	type execResult struct {
		resp *createPoolExecuteResponse
		err  error
	}
	resultCh := make(chan execResult, 1)
	ok := txexec.Default().TryRunWallet(plan.ctx.wallet.Address, func() {
		resp, err := s.executeCreatePoolPlan(plan)
		resultCh <- execResult{resp: resp, err: err}
	})
	if !ok {
		http.Error(w, "wallet is busy with another transaction", http.StatusConflict)
		return
	}

	result := <-resultCh
	if result.err != nil {
		http.Error(w, result.err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result.resp)
}
