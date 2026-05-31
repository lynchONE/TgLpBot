package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/convert"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"TgLpBot/service/pricing"
	userSvc "TgLpBot/service/user"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
)

const (
	hardRejectMinLiquidityUSD = 200.0
	softWarnMaxLiquidityUSD   = 1000.0
)

type OpenPositionRiskOptions struct {
	AckLiquidityRisk    bool
	RequireLiquidityAck bool
}

type openPositionGuardState struct {
	PoolID         string
	Chain          string
	Version        string
	Token0         common.Address
	Token1         common.Address
	Token0Decimals int
	Token1Decimals int
	SqrtPriceX96   *big.Int
	RawLiquidity   *big.Int
	LiquidityUSD   float64
}

type dexScreenerGuardLiquidity struct {
	USD float64 `json:"usd"`
}

type dexScreenerGuardPair struct {
	PairAddress string                    `json:"pairAddress"`
	Liquidity   dexScreenerGuardLiquidity `json:"liquidity"`
}

type dexScreenerGuardResponse struct {
	Pairs []dexScreenerGuardPair `json:"pairs"`
}

func firstPositiveLiquidityUSD(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func normalizeLiquidityGuardHex(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		value = value[2:]
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return "0x" + value
}

func poolSnapshotLiquidityUSD(row *models.Pool) float64 {
	liquidityUSD, _ := poolSnapshotLiquidityUSDWithSource(row)
	return liquidityUSD
}

func poolSnapshotLiquidityUSDWithSource(row *models.Pool) (float64, string) {
	if row == nil {
		return 0, ""
	}
	switch {
	case row.ActiveLiquidityUSD > 0:
		return row.ActiveLiquidityUSD, "pool_snapshot.active_liquidity_usd"
	case row.CurrentPoolValue > 0:
		return row.CurrentPoolValue, "pool_snapshot.current_pool_value"
	case row.ReserveInUSD > 0:
		return row.ReserveInUSD, "pool_snapshot.reserve_in_usd"
	default:
		return 0, ""
	}
}

func normalizeDexScreenerGuardChain(chain string) string {
	switch strings.ToLower(strings.TrimSpace(chain)) {
	case "", "bsc", "bnb":
		return "bsc"
	case "eth", "ethereum":
		return "ethereum"
	default:
		return strings.ToLower(strings.TrimSpace(chain))
	}
}

func readPoolSnapshotLiquidityUSD(chain string, poolID string) float64 {
	liquidityUSD, _, _ := readPoolSnapshotLiquidityUSDWithSource(chain, poolID)
	return liquidityUSD
}

func readPoolSnapshotLiquidityUSDWithSource(chain string, poolID string) (float64, string, error) {
	if database.DB == nil {
		return 0, "", nil
	}
	normalizedChain := config.NormalizeChain(chain)
	normalizedPoolID := normalizeLiquidityGuardHex(poolID)
	if normalizedChain == "" || normalizedPoolID == "" {
		return 0, "", nil
	}

	var row models.Pool
	err := database.DB.
		Where("chain = ? AND address = ?", normalizedChain, strings.ToLower(normalizedPoolID)).
		Order("updated_at DESC").
		First(&row).Error
	if err != nil {
		if err != nil && err != gorm.ErrRecordNotFound {
			log.Printf("[Liquidity] guard: load pool snapshot failed: chain=%s pool=%s err=%v", normalizedChain, normalizedPoolID, err)
			return 0, "", err
		}
		return 0, "", nil
	}
	liquidityUSD, source := poolSnapshotLiquidityUSDWithSource(&row)
	return liquidityUSD, source, nil
}

func fetchDexScreenerLiquidityUSD(chain string, poolID string) (float64, error) {
	normalizedPoolID := normalizeLiquidityGuardHex(poolID)
	if normalizedPoolID == "" || !common.IsHexAddress(normalizedPoolID) {
		return 0, nil
	}

	endpoint := fmt.Sprintf(
		"https://api.dexscreener.com/latest/dex/pairs/%s/%s",
		url.PathEscape(normalizeDexScreenerGuardChain(chain)),
		url.PathEscape(normalizedPoolID),
	)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("dexscreener http %d", resp.StatusCode)
	}

	var payload dexScreenerGuardResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, err
	}
	for _, pair := range payload.Pairs {
		if normalizeLiquidityGuardHex(pair.PairAddress) != normalizedPoolID {
			continue
		}
		if pair.Liquidity.USD > 0 {
			return pair.Liquidity.USD, nil
		}
	}
	for _, pair := range payload.Pairs {
		if pair.Liquidity.USD > 0 {
			return pair.Liquidity.USD, nil
		}
	}
	return 0, nil
}

func resolvePoolLiquidityUSD(chain string, poolID string) (float64, error) {
	liquidityUSD, _, err := ResolvePoolLiquidityUSDWithSource(chain, poolID)
	return liquidityUSD, err
}

func ResolvePoolLiquidityUSDWithSource(chain string, poolID string) (float64, string, error) {
	if liquidityUSD, source, err := readPoolSnapshotLiquidityUSDWithSource(chain, poolID); err != nil {
		return 0, source, err
	} else if liquidityUSD > 0 {
		return liquidityUSD, source, nil
	}
	liquidityUSD, err := fetchDexScreenerLiquidityUSD(chain, poolID)
	if err != nil {
		return 0, "dexscreener", err
	}
	if liquidityUSD > 0 {
		return liquidityUSD, "dexscreener", nil
	}
	return 0, "", nil
}

func tokenDecimalsWithFallback(client *ethclient.Client, token common.Address, fallback int) int {
	if token == (common.Address{}) {
		return fallback
	}
	decimals, err := blockchain.GetTokenDecimalsWithClient(client, token)
	if err != nil {
		return fallback
	}
	if decimals <= 0 {
		return fallback
	}
	return int(decimals)
}

func priceFromSqrtRatioX96(sqrtPriceX96 *big.Int, token0Decimals int, token1Decimals int) (float64, error) {
	if sqrtPriceX96 == nil || sqrtPriceX96.Sign() <= 0 {
		return 0, fmt.Errorf("sqrtPriceX96 is empty")
	}

	precision := uint(256)
	sqrtFloat := new(big.Float).SetPrec(precision).SetInt(sqrtPriceX96)
	q96 := new(big.Float).SetPrec(precision).SetInt(new(big.Int).Lsh(big.NewInt(1), 96))
	ratio := new(big.Float).SetPrec(precision).Quo(sqrtFloat, q96)
	price := new(big.Float).SetPrec(precision).Mul(ratio, ratio)

	decimalDiff := token0Decimals - token1Decimals
	if decimalDiff > 0 {
		price.Mul(price, new(big.Float).SetFloat64(math.Pow10(decimalDiff)))
	} else if decimalDiff < 0 {
		price.Quo(price, new(big.Float).SetFloat64(math.Pow10(-decimalDiff)))
	}

	out, _ := price.Float64()
	if math.IsNaN(out) || math.IsInf(out, 0) || out <= 0 {
		return 0, fmt.Errorf("invalid pool price")
	}
	return out, nil
}

func amountToFloat(amount *big.Int, decimals int) float64 {
	if amount == nil || amount.Sign() <= 0 {
		return 0
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	value, _ := new(big.Float).Quo(new(big.Float).SetInt(amount), new(big.Float).SetInt(scale)).Float64()
	return value
}

func quotePriceToken1PerToken0(
	swapTokenIn common.Address,
	swapAmountIn *big.Int,
	okxExpectedOut *big.Int,
	token0 common.Address,
	token1 common.Address,
	token0Decimals int,
	token1Decimals int,
) float64 {
	if swapAmountIn == nil || swapAmountIn.Sign() <= 0 || okxExpectedOut == nil || okxExpectedOut.Sign() <= 0 {
		return 0
	}
	switch {
	case swapTokenIn == token0:
		inHuman := amountToFloat(swapAmountIn, token0Decimals)
		outHuman := amountToFloat(okxExpectedOut, token1Decimals)
		if inHuman > 0 && outHuman > 0 {
			return outHuman / inHuman
		}
	case swapTokenIn == token1:
		inHuman := amountToFloat(swapAmountIn, token1Decimals)
		outHuman := amountToFloat(okxExpectedOut, token0Decimals)
		if inHuman > 0 && outHuman > 0 {
			return inHuman / outHuman
		}
	}
	return 0
}

func isStableEntryToken(symbol string, token common.Address, cc config.ChainConfig) bool {
	tokenHex := strings.ToLower(token.Hex())
	if tokenHex != "" {
		for _, candidate := range []string{cc.StableAddress, cc.USDTAddress, cc.USDCAddress, cc.BUSDAddress} {
			addr := strings.ToLower(strings.TrimSpace(candidate))
			if addr != "" && tokenHex == addr {
				return true
			}
		}
	}

	switch strings.ToUpper(strings.TrimSpace(symbol)) {
	case "USDT", "USDC", "BUSD", "DAI", "FDUSD", "USDD", "FRAX":
		return true
	default:
		return false
	}
}

func tokenPriceUSD(chain string, token common.Address, symbol string, cc config.ChainConfig) (float64, error) {
	if token == (common.Address{}) {
		price := pricing.GetNativePriceUSD(chain)
		if price > 0 {
			return price, nil
		}
		if wrapped, ok := wrappedNativeAddress(cc); ok {
			token = wrapped
		} else {
			return 0, fmt.Errorf("native token price unavailable")
		}
	}
	if isStableEntryToken(symbol, token, cc) {
		return 1.0, nil
	}

	prices, err := pricing.DefaultTokenPriceService().GetUSDPrices(chain, []string{token.Hex()})
	if err != nil {
		return 0, err
	}
	price := prices[strings.ToLower(token.Hex())]
	if price <= 0 {
		return 0, fmt.Errorf("missing usd price for %s", token.Hex())
	}
	return price, nil
}

func tokenBudgetUnits(client *ethclient.Client, chain string, token common.Address, symbol string, cc config.ChainConfig, targetUSD float64) (*big.Int, error) {
	if targetUSD <= 0 {
		return big.NewInt(0), nil
	}
	decimals := tokenDecimalsWithFallback(client, token, cc.StableDecimals)
	priceUSD, err := tokenPriceUSD(chain, token, symbol, cc)
	if err != nil {
		return nil, err
	}
	if priceUSD <= 0 {
		return nil, fmt.Errorf("invalid usd price for %s", token.Hex())
	}

	tokenAmount := targetUSD / priceUSD
	if tokenAmount <= 0 {
		return big.NewInt(0), nil
	}
	return convert.FloatToUnits(tokenAmount, decimals)
}

func tokenValueInStableUnits(client *ethclient.Client, chain string, token common.Address, symbol string, cc config.ChainConfig, amount *big.Int) (*big.Int, error) {
	if amount == nil || amount.Sign() <= 0 {
		return big.NewInt(0), nil
	}
	decimals := tokenDecimalsWithFallback(client, token, cc.StableDecimals)
	human := amountToFloat(amount, decimals)
	if human <= 0 {
		return big.NewInt(0), nil
	}
	priceUSD, err := tokenPriceUSD(chain, token, symbol, cc)
	if err != nil {
		return nil, err
	}
	return convert.FloatToUnits(human*priceUSD, cc.StableDecimals)
}

func hardMinLiquidityUSD(configuredMin float64) float64 {
	if configuredMin > hardRejectMinLiquidityUSD {
		return configuredMin
	}
	return hardRejectMinLiquidityUSD
}

func evaluateLiquidityRisk(liquidityUSD float64, configuredMin float64, options OpenPositionRiskOptions) *ZapSafetyError {
	minLiquidityUSD := hardMinLiquidityUSD(configuredMin)
	if liquidityUSD <= 0 {
		return &ZapSafetyError{
			Code:            "pool_liquidity_unknown",
			Reason:          "无法确认该池子的当前流动性，已取消开仓",
			LiquidityUSD:    liquidityUSD,
			MinLiquidityUSD: minLiquidityUSD,
		}
	}
	if liquidityUSD < minLiquidityUSD {
		return &ZapSafetyError{
			Code:            "pool_liquidity_too_low",
			Reason:          fmt.Sprintf("该池子当前流动性仅 %.2fU，低于允许开仓门槛 %.2fU", liquidityUSD, minLiquidityUSD),
			LiquidityUSD:    liquidityUSD,
			MinLiquidityUSD: minLiquidityUSD,
		}
	}
	if liquidityUSD >= softWarnMaxLiquidityUSD {
		return nil
	}
	if options.RequireLiquidityAck && !options.AckLiquidityRisk {
		return &ZapSafetyError{
			Code:            "pool_liquidity_warning",
			Reason:          fmt.Sprintf("该池子当前流动性为 %.2fU，请确认低流动性风险后再继续开仓", liquidityUSD),
			LiquidityUSD:    liquidityUSD,
			MinLiquidityUSD: minLiquidityUSD,
			RiskAckRequired: true,
		}
	}
	return nil
}

func buildSoftLiquidityWarning(liquidityUSD float64, configuredMin float64) *ZapSafetyError {
	minLiquidityUSD := hardMinLiquidityUSD(configuredMin)
	if liquidityUSD <= 0 || liquidityUSD < minLiquidityUSD || liquidityUSD >= softWarnMaxLiquidityUSD {
		return nil
	}
	return &ZapSafetyError{
		Code:            "pool_liquidity_warning",
		Reason:          fmt.Sprintf("该池子当前流动性为 %.2fU，属于低流动性池，请留意滑点与成交波动", liquidityUSD),
		LiquidityUSD:    liquidityUSD,
		MinLiquidityUSD: minLiquidityUSD,
	}
}

func readOpenPositionGuardState(task *models.StrategyTask) (*openPositionGuardState, config.ChainConfig, *ethclient.Client, *models.ZapSafetyConfig, error) {
	if task == nil {
		return nil, config.ChainConfig{}, nil, nil, fmt.Errorf("task is nil")
	}
	normalizedChain := config.NormalizeChain(task.Chain)
	exec, err := chainexec.GetEVM(normalizedChain)
	if err != nil {
		return nil, config.ChainConfig{}, nil, nil, err
	}
	client := exec.Client()
	if client == nil {
		return nil, config.ChainConfig{}, nil, nil, fmt.Errorf("blockchain client not initialized")
	}
	cc := exec.Config()
	sysConfigService := userSvc.NewSystemConfigService()
	safety, err := sysConfigService.GetZapSafetyConfig()
	if err != nil {
		return nil, config.ChainConfig{}, nil, nil, err
	}

	token0, token1, err := NewLiquidityService().resolveTaskTokenAddresses(task)
	if err != nil {
		return nil, config.ChainConfig{}, nil, nil, err
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	poolID := strings.TrimSpace(task.PoolId)
	state := &openPositionGuardState{
		PoolID:  poolID,
		Chain:   normalizedChain,
		Version: version,
		Token0:  token0,
		Token1:  token1,
	}

	switch version {
	case "v4":
		state.SqrtPriceX96, _, err = blockchain.GetUniswapV4PoolSlot0ViaStateView(
			common.HexToAddress(cc.UniswapV4StateViewAddress),
			common.HexToAddress(cc.UniswapV4PoolManagerAddress),
			poolID,
		)
		if err != nil {
			return nil, config.ChainConfig{}, nil, nil, fmt.Errorf("读取 V4 池子价格失败：%w", err)
		}
		state.RawLiquidity, err = blockchain.GetUniswapV4PoolLiquidityViaStateView(
			common.HexToAddress(cc.UniswapV4StateViewAddress),
			common.HexToAddress(cc.UniswapV4PoolManagerAddress),
			poolID,
		)
		if err != nil {
			return nil, config.ChainConfig{}, nil, nil, fmt.Errorf("读取 V4 池子流动性失败：%w", err)
		}
	default:
		if !common.IsHexAddress(poolID) {
			return nil, config.ChainConfig{}, nil, nil, fmt.Errorf("invalid V3 pool address")
		}
		poolAddr := common.HexToAddress(poolID)
		state.SqrtPriceX96, _, err = blockchain.GetV3PoolSlot0WithClient(client, poolAddr)
		if err != nil {
			return nil, config.ChainConfig{}, nil, nil, fmt.Errorf("读取 V3 池子价格失败：%w", err)
		}
		state.RawLiquidity, err = blockchain.GetV3PoolLiquidityWithClient(client, poolAddr)
		if err != nil {
			return nil, config.ChainConfig{}, nil, nil, fmt.Errorf("读取 V3 池子流动性失败：%w", err)
		}
	}

	state.Token0Decimals = tokenDecimalsWithFallback(client, token0, 18)
	state.Token1Decimals = tokenDecimalsWithFallback(client, token1, 18)
	state.LiquidityUSD, err = resolvePoolLiquidityUSD(normalizedChain, poolID)
	if err != nil {
		log.Printf("[Liquidity] guard: resolve liquidity USD failed: chain=%s pool=%s err=%v", normalizedChain, poolID, err)
	}
	return state, cc, client, safety, nil
}

func marketPriceToken1PerToken0(chain string, token0 common.Address, token1 common.Address, token0Symbol string, token1Symbol string, cc config.ChainConfig) (float64, error) {
	token0Price, err := tokenPriceUSD(chain, token0, token0Symbol, cc)
	if err != nil {
		return 0, err
	}
	token1Price, err := tokenPriceUSD(chain, token1, token1Symbol, cc)
	if err != nil {
		return 0, err
	}
	if token0Price <= 0 || token1Price <= 0 {
		return 0, fmt.Errorf("invalid market price")
	}
	return token0Price / token1Price, nil
}

func evaluatePriceDeviation(
	task *models.StrategyTask,
	state *openPositionGuardState,
	cc config.ChainConfig,
	safety *models.ZapSafetyConfig,
) *ZapSafetyError {
	if task == nil || state == nil || safety == nil || safety.PriceDeviationMaxPercent <= 0 {
		return nil
	}

	poolPrice, err := priceFromSqrtRatioX96(state.SqrtPriceX96, state.Token0Decimals, state.Token1Decimals)
	if err != nil {
		return &ZapSafetyError{
			Code:                     "pool_price_check_failed",
			Reason:                   err.Error(),
			LiquidityUSD:             state.LiquidityUSD,
			PriceDeviationMaxPercent: safety.PriceDeviationMaxPercent,
		}
	}

	marketPrice, err := marketPriceToken1PerToken0(
		state.Chain,
		state.Token0,
		state.Token1,
		task.Token0Symbol,
		task.Token1Symbol,
		cc,
	)
	if err != nil {
		log.Printf("[Liquidity] guard: market price lookup skipped: chain=%s pool=%s err=%v", state.Chain, state.PoolID, err)
		return nil
	}
	if poolPrice <= 0 || marketPrice <= 0 {
		return nil
	}

	deviation := math.Abs(marketPrice-poolPrice) / poolPrice * 100.0
	if deviation <= safety.PriceDeviationMaxPercent {
		return nil
	}

	return &ZapSafetyError{
		Code:                     "pool_price_deviation_too_high",
		Reason:                   fmt.Sprintf("池子当前价格与市场价格偏差 %.2f%%，已超过阈值 %.2f%%", deviation, safety.PriceDeviationMaxPercent),
		LiquidityUSD:             state.LiquidityUSD,
		PriceDeviationPercent:    deviation,
		PriceDeviationMaxPercent: safety.PriceDeviationMaxPercent,
	}
}

func (s *LiquidityService) CheckOpenPositionSafety(task *models.StrategyTask, options OpenPositionRiskOptions) error {
	state, cc, _, safety, err := readOpenPositionGuardState(task)
	if err != nil {
		return &ZapSafetyError{
			Code:   "pool_state_read_failed",
			Reason: err.Error(),
		}
	}

	if err := ensurePoolHasLiquidity(state.Version, state.RawLiquidity); err != nil {
		var safetyErr *ZapSafetyError
		if ok := errorAs(err, &safetyErr); ok && safetyErr != nil {
			safetyErr.LiquidityUSD = state.LiquidityUSD
			safetyErr.MinLiquidityUSD = hardMinLiquidityUSD(safety.MinPoolLiquidityUSD)
		}
		return err
	}

	if err := evaluateLiquidityRisk(state.LiquidityUSD, safety.MinPoolLiquidityUSD, options); err != nil {
		return err
	}

	if devErr := evaluatePriceDeviation(task, state, cc, safety); devErr != nil {
		return devErr
	}
	return nil
}

// CheckResult represents a single check item result for the open position checklist.
type CheckResult struct {
	Key    string                 `json:"key"`
	Status string                 `json:"status"` // "pass", "warn", "fail"
	Label  string                 `json:"label"`
	Detail string                 `json:"detail,omitempty"`
	Value  *float64               `json:"value,omitempty"`
	Extra  map[string]interface{} `json:"extra,omitempty"`
}

// CollectOpenPositionChecks runs all safety checks and returns results for each item
// instead of returning on the first error.
func (s *LiquidityService) CollectOpenPositionChecks(task *models.StrategyTask, options OpenPositionRiskOptions) ([]CheckResult, error) {
	state, cc, _, safety, err := readOpenPositionGuardState(task)
	if err != nil {
		return nil, fmt.Errorf("读取池子状态失败: %w", err)
	}

	var checks []CheckResult

	// 1. Pool raw liquidity (on-chain zero check)
	rawLiqErr := ensurePoolHasLiquidity(state.Version, state.RawLiquidity)
	if rawLiqErr != nil {
		checks = append(checks, CheckResult{
			Key:    "liquidity",
			Status: "fail",
			Label:  "池子流动性",
			Detail: rawLiqErr.Error(),
		})
		// If pool has zero liquidity, skip remaining checks
		return checks, nil
	}

	// 2. Liquidity USD check
	liqErr := evaluateLiquidityRisk(state.LiquidityUSD, safety.MinPoolLiquidityUSD, options)
	if liqErr == nil {
		if warn := buildSoftLiquidityWarning(state.LiquidityUSD, safety.MinPoolLiquidityUSD); warn != nil {
			liqVal := state.LiquidityUSD
			checks = append(checks, CheckResult{
				Key:    "liquidity",
				Status: "warn",
				Label:  "池子流动性",
				Detail: warn.Reason,
				Value:  &liqVal,
				Extra: map[string]interface{}{
					"liquidity_usd": warn.LiquidityUSD,
				},
			})
		} else {
			liqVal := state.LiquidityUSD
			checks = append(checks, CheckResult{
				Key:    "liquidity",
				Status: "pass",
				Label:  "池子流动性",
				Detail: fmt.Sprintf("TVL $%.0f", state.LiquidityUSD),
				Value:  &liqVal,
			})
		}
	} else {
		liqVal := state.LiquidityUSD
		item := CheckResult{
			Key:   "liquidity",
			Label: "池子流动性",
			Value: &liqVal,
			Extra: map[string]interface{}{
				"liquidity_usd": liqErr.LiquidityUSD,
			},
		}
		if liqErr.RiskAckRequired {
			item.Status = "warn"
			item.Detail = liqErr.Reason
			item.Extra["risk_ack_required"] = true
		} else {
			item.Status = "fail"
			item.Detail = liqErr.Reason
		}
		checks = append(checks, item)
	}

	// 3. Price deviation check
	devErr := evaluatePriceDeviation(task, state, cc, safety)
	if devErr == nil {
		checks = append(checks, CheckResult{
			Key:    "price_deviation",
			Status: "pass",
			Label:  "价格偏差",
			Detail: "正常",
		})
	} else {
		devVal := devErr.PriceDeviationPercent
		item := CheckResult{
			Key:    "price_deviation",
			Label:  "价格偏差",
			Detail: devErr.Reason,
			Value:  &devVal,
			Extra: map[string]interface{}{
				"price_deviation_max_percent": devErr.PriceDeviationMaxPercent,
			},
		}
		// Price deviation is always a hard fail
		item.Status = "fail"
		checks = append(checks, item)
	}

	return checks, nil
}

func errorAs(err error, target interface{}) bool {
	switch v := target.(type) {
	case **ZapSafetyError:
		if err == nil {
			return false
		}
		typed, ok := err.(*ZapSafetyError)
		if ok && typed != nil {
			*v = typed
			return true
		}
	}
	return false
}
