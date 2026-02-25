package pricing

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const stableUnknown = -1

var stableSymbols = map[string]struct{}{
	"USDT": {},
	"USDC": {},
	"BUSD": {},
	"DAI":  {},
}

// StableSideFromTask returns which token is considered the stable quote.
// -1: unknown, 0: token0, 1: token1.
func StableSideFromTask(task *models.StrategyTask) int {
	return stableSideFromTask(task)
}

// TickPercentagesFromStablePercentages converts stable-price range percentages into tick-price percentages.
// stableLowerPct/stableUpperPct are in stable price terms: price down/up from current (e.g. 1 means -1%).
// Returned tickLowerPct/tickUpperPct are in Uniswap tick price (token1/token0) terms.
func TickPercentagesFromStablePercentages(task *models.StrategyTask, stableLowerPct, stableUpperPct float64) (tickLowerPct, tickUpperPct float64) {
	if stableLowerPct <= 0 || stableUpperPct <= 0 {
		return 0, 0
	}
	if stableLowerPct >= 100 || stableUpperPct >= 100 {
		return 0, 0
	}

	side := stableSideFromTask(task)
	if side != 0 {
		// Stable is token1 (or unknown): stable price direction matches tick price direction.
		return stableLowerPct, stableUpperPct
	}

	// Stable is token0: stable price = 1 / tickPrice.
	// When stable price goes UP by U, tickPrice goes DOWN by U/(100+U).
	// When stable price goes DOWN by L, tickPrice goes UP by L/(100-L).
	tickLowerPct = (stableUpperPct / (100.0 + stableUpperPct)) * 100.0
	tickUpperPct = (stableLowerPct / (100.0 - stableLowerPct)) * 100.0

	if tickLowerPct <= 0 || tickUpperPct <= 0 {
		return 0, 0
	}
	return tickLowerPct, tickUpperPct
}

// StablePercentagesFromTickPercentages converts tick-price range percentages into stable-price percentages.
// tickLowerPct/tickUpperPct are in Uniswap tick price (token1/token0) terms: price down/up from current.
// Returned stableLowerPct/stableUpperPct are in stable price terms when a stable side is known.
func StablePercentagesFromTickPercentages(task *models.StrategyTask, tickLowerPct, tickUpperPct float64) (stableLowerPct, stableUpperPct float64) {
	if tickLowerPct <= 0 || tickUpperPct <= 0 {
		return 0, 0
	}

	side := stableSideFromTask(task)
	if side != 0 {
		// Stable is token1 (or unknown): stable price direction matches tick price direction.
		return tickLowerPct, tickUpperPct
	}

	// Stable is token0: stable price = 1 / tickPrice.
	// When tickPrice goes UP by u, stable price goes DOWN by u/(100+u).
	// When tickPrice goes DOWN by d, stable price goes UP by d/(100-d).
	stableLowerPct = (tickUpperPct / (100.0 + tickUpperPct)) * 100.0
	if tickLowerPct >= 100 {
		stableUpperPct = 0
	} else {
		stableUpperPct = (tickLowerPct / (100.0 - tickLowerPct)) * 100.0
	}

	if stableLowerPct <= 0 || stableUpperPct <= 0 {
		return 0, 0
	}
	return stableLowerPct, stableUpperPct
}

func isStableSymbol(sym string) bool {
	sym = strings.ToUpper(strings.TrimSpace(sym))
	_, ok := stableSymbols[sym]
	return ok
}

// IsStableSymbol exposes stable-coin symbol checks to other packages.
func IsStableSymbol(sym string) bool {
	return isStableSymbol(sym)
}

func isStableAddress(chain string, addr string) bool {
	if config.AppConfig == nil {
		return false
	}
	chain = config.NormalizeChain(chain)
	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok {
		return false
	}
	addr = strings.ToLower(strings.TrimSpace(addr))
	if !common.IsHexAddress(addr) {
		return false
	}
	stables := []string{
		cc.StableAddress,
		cc.USDCAddress,
		cc.BUSDAddress,
	}
	for _, stable := range stables {
		stable = strings.ToLower(strings.TrimSpace(stable))
		if stable == "" || !common.IsHexAddress(stable) {
			continue
		}
		if addr == stable {
			return true
		}
	}
	return false
}

// IsStableAddress exposes stable-coin address checks to other packages.
func IsStableAddress(chain string, addr string) bool {
	return isStableAddress(chain, addr)
}

const DefaultTokenDecimals = 18

const (
	decimalsCacheTTL  = 24 * time.Hour
	decimalsErrorTTL  = 5 * time.Minute
	poolTokenCacheTTL = 24 * time.Hour
	poolTokenErrorTTL = 5 * time.Minute
)

type cachedDecimals struct {
	value   int
	expires time.Time
}

var decimalsCache = struct {
	mu     sync.RWMutex
	values map[string]cachedDecimals
}{
	values: make(map[string]cachedDecimals),
}

type cachedPoolTokens struct {
	token0  string
	token1  string
	expires time.Time
}

var poolTokenCache = struct {
	mu     sync.RWMutex
	values map[string]cachedPoolTokens
}{
	values: make(map[string]cachedPoolTokens),
}

// stableSideFromTask returns which token is the stable quote.
// -1: unknown, 0: token0, 1: token1.
func stableSideFromTask(task *models.StrategyTask) int {
	if task == nil {
		return stableUnknown
	}

	chain := config.NormalizeChain(task.Chain)
	token0Addr, token1Addr := resolveTokenAddresses(task)
	if isStableAddress(chain, token0Addr) {
		return 0
	}
	if isStableAddress(chain, token1Addr) {
		return 1
	}

	sym0 := strings.ToUpper(strings.TrimSpace(task.Token0Symbol))
	sym1 := strings.ToUpper(strings.TrimSpace(task.Token1Symbol))
	if isStableSymbol(sym0) {
		return 0
	}
	if isStableSymbol(sym1) {
		return 1
	}
	return stableUnknown
}

func GetTokenDecimals(chain string, addr string) int {
	chain = config.NormalizeChain(chain)
	addr = strings.TrimSpace(addr)
	if !common.IsHexAddress(addr) {
		return DefaultTokenDecimals
	}
	key := chain + "|" + strings.ToLower(addr)
	now := time.Now()

	decimalsCache.mu.RLock()
	if c, ok := decimalsCache.values[key]; ok && c.expires.After(now) {
		decimalsCache.mu.RUnlock()
		return c.value
	}
	decimalsCache.mu.RUnlock()

	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return DefaultTokenDecimals
	}

	decimals, err := blockchain.GetTokenDecimalsWithClient(client, common.HexToAddress(addr))
	if err != nil || decimals == 0 {
		decimalsCache.mu.Lock()
		decimalsCache.values[key] = cachedDecimals{
			value:   DefaultTokenDecimals,
			expires: now.Add(decimalsErrorTTL),
		}
		decimalsCache.mu.Unlock()
		return DefaultTokenDecimals
	}

	value := int(decimals)
	if value <= 0 {
		value = DefaultTokenDecimals
	}
	decimalsCache.mu.Lock()
	decimalsCache.values[key] = cachedDecimals{
		value:   value,
		expires: now.Add(decimalsCacheTTL),
	}
	decimalsCache.mu.Unlock()
	return value
}

func resolveTokenAddresses(task *models.StrategyTask) (string, string) {
	if task == nil {
		return "", ""
	}

	chain := config.NormalizeChain(task.Chain)
	token0Addr := strings.TrimSpace(task.Token0Address)
	token1Addr := strings.TrimSpace(task.Token1Address)
	valid0 := common.IsHexAddress(token0Addr)
	valid1 := common.IsHexAddress(token1Addr)
	if valid0 && valid1 {
		return token0Addr, token1Addr
	}

	version := strings.ToLower(strings.TrimSpace(task.PoolVersion))
	if version == "v4" {
		return token0Addr, token1Addr
	}

	poolID := strings.TrimSpace(task.PoolId)
	if !common.IsHexAddress(poolID) {
		return token0Addr, token1Addr
	}
	client, _, err := blockchain.GetEVMClient(chain)
	if err != nil {
		return token0Addr, token1Addr
	}

	now := time.Now()
	cacheKey := chain + "|" + strings.ToLower(poolID)
	poolTokenCache.mu.RLock()
	if c, ok := poolTokenCache.values[cacheKey]; ok && c.expires.After(now) {
		poolTokenCache.mu.RUnlock()
		return c.token0, c.token1
	}
	poolTokenCache.mu.RUnlock()

	token0, token1, err := blockchain.GetV3PoolTokensWithClient(client, common.HexToAddress(poolID))
	if err != nil {
		poolTokenCache.mu.Lock()
		poolTokenCache.values[cacheKey] = cachedPoolTokens{
			token0:  token0Addr,
			token1:  token1Addr,
			expires: now.Add(poolTokenErrorTTL),
		}
		poolTokenCache.mu.Unlock()
		return token0Addr, token1Addr
	}

	resolved0 := token0.Hex()
	resolved1 := token1.Hex()
	if !common.IsHexAddress(resolved0) {
		resolved0 = token0Addr
	}
	if !common.IsHexAddress(resolved1) {
		resolved1 = token1Addr
	}

	poolTokenCache.mu.Lock()
	poolTokenCache.values[cacheKey] = cachedPoolTokens{
		token0:  resolved0,
		token1:  resolved1,
		expires: now.Add(poolTokenCacheTTL),
	}
	poolTokenCache.mu.Unlock()

	return resolved0, resolved1
}

// PriceDirectionFromTicks returns out-of-range direction in stable price terms.
// isAbove/isBelow are based on raw ticks; priceUp/priceDown map to stable price direction.
func PriceDirectionFromTicks(task *models.StrategyTask, tickLower, tickUpper, currentTick int) (isAbove bool, isBelow bool, priceUp bool, priceDown bool) {
	isAbove = currentTick > tickUpper
	isBelow = currentTick < tickLower
	priceUp = isAbove
	priceDown = isBelow

	if stableSideFromTask(task) == 0 {
		priceUp = isBelow
		priceDown = isAbove
	}
	return
}

type priceDisplayContext struct {
	dec0        int
	dec1        int
	invert      bool
	baseSymbol  string
	quoteSymbol string
	ok          bool
}

func getPriceDisplayContext(task *models.StrategyTask) priceDisplayContext {
	if task == nil {
		return priceDisplayContext{}
	}

	chain := config.NormalizeChain(task.Chain)
	token0Addr, token1Addr := resolveTokenAddresses(task)
	dec0 := GetTokenDecimals(chain, token0Addr)
	dec1 := GetTokenDecimals(chain, token1Addr)

	side := stableSideFromTask(task)
	base := strings.TrimSpace(task.Token0Symbol)
	quote := strings.TrimSpace(task.Token1Symbol)
	invert := false

	switch side {
	case 0:
		invert = true
		base = strings.TrimSpace(task.Token1Symbol)
		quote = strings.TrimSpace(task.Token0Symbol)
	case 1:
		invert = false
		base = strings.TrimSpace(task.Token0Symbol)
		quote = strings.TrimSpace(task.Token1Symbol)
	default:
		invert = false
		base = strings.TrimSpace(task.Token0Symbol)
		quote = strings.TrimSpace(task.Token1Symbol)
	}

	if strings.TrimSpace(base) == "" {
		base = "-"
	}
	if strings.TrimSpace(quote) == "" {
		quote = "-"
	}

	return priceDisplayContext{
		dec0:        dec0,
		dec1:        dec1,
		invert:      invert,
		baseSymbol:  base,
		quoteSymbol: quote,
		ok:          true,
	}
}

func priceFromTickWithDecimals(tick int, dec0, dec1 int) float64 {
	raw := math.Pow(1.0001, float64(tick))
	if !isValidPrice(raw) {
		return 0
	}
	scale := math.Pow(10, float64(dec0-dec1))
	adjusted := raw * scale
	if !isValidPrice(adjusted) {
		return 0
	}
	return adjusted
}

func BuildPriceDisplay(task *models.StrategyTask, tick int) (float64, string, string, bool) {
	ctx := getPriceDisplayContext(task)
	if !ctx.ok {
		return 0, "", "", false
	}

	raw := priceFromTickWithDecimals(tick, ctx.dec0, ctx.dec1)
	if !isValidPrice(raw) {
		return 0, "", "", false
	}

	price := raw
	if ctx.invert {
		inv, ok := invertPrice(raw)
		if !ok {
			return 0, "", "", false
		}
		price = inv
	}

	return price, ctx.baseSymbol, ctx.quoteSymbol, true
}

func BuildRangeDisplay(task *models.StrategyTask, tickLower, tickUpper int) (float64, float64, string, string, bool) {
	ctx := getPriceDisplayContext(task)
	if !ctx.ok {
		return 0, 0, "", "", false
	}

	rawLower := priceFromTickWithDecimals(tickLower, ctx.dec0, ctx.dec1)
	rawUpper := priceFromTickWithDecimals(tickUpper, ctx.dec0, ctx.dec1)
	if !isValidPrice(rawLower) || !isValidPrice(rawUpper) {
		return 0, 0, "", "", false
	}

	lower := rawLower
	upper := rawUpper
	if ctx.invert {
		lowerInv, ok := invertPrice(rawUpper)
		if !ok {
			return 0, 0, "", "", false
		}
		upperInv, ok := invertPrice(rawLower)
		if !ok {
			return 0, 0, "", "", false
		}
		lower = lowerInv
		upper = upperInv
	}

	if lower > upper {
		lower, upper = upper, lower
	}

	return lower, upper, ctx.baseSymbol, ctx.quoteSymbol, true
}

type PriceRangeDisplay struct {
	Current     float64
	Lower       float64
	Upper       float64
	BaseSymbol  string
	QuoteSymbol string
	HasCurrent  bool
	HasRange    bool
}

func BuildPriceRangeDisplay(task *models.StrategyTask, tickLower, tickUpper, currentTick int) PriceRangeDisplay {
	display := PriceRangeDisplay{}

	current, base, quote, okCurrent := BuildPriceDisplay(task, currentTick)
	if okCurrent {
		display.Current = current
		display.BaseSymbol = base
		display.QuoteSymbol = quote
		display.HasCurrent = true
	}

	lower, upper, base, quote, okRange := BuildRangeDisplay(task, tickLower, tickUpper)
	if okRange {
		display.Lower = lower
		display.Upper = upper
		display.BaseSymbol = base
		display.QuoteSymbol = quote
		display.HasRange = true
	}

	return display
}

func invertPrice(value float64) (float64, bool) {
	if !isValidPrice(value) {
		return 0, false
	}
	inv := 1.0 / value
	if !isValidPrice(inv) {
		return 0, false
	}
	return inv, true
}

func isValidPrice(value float64) bool {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	return true
}

func FormatPriceValue(price float64) string {
	if !isValidPrice(price) {
		return "--"
	}

	sign := ""
	if price < 0 {
		sign = "-"
		price = -price
	}

	raw := strconv.FormatFloat(price, 'f', -1, 64)
	if !strings.Contains(raw, ".") {
		return sign + raw
	}

	parts := strings.SplitN(raw, ".", 2)
	intPart := parts[0]
	fracPart := strings.TrimRight(parts[1], "0")
	if fracPart == "" {
		return sign + intPart
	}

	// 保留 4 位有效非零数字（原来是 2 位）
	nonZero := 0
	cut := len(fracPart)
	for i := 0; i < len(fracPart); i++ {
		if fracPart[i] != '0' {
			nonZero++
			if nonZero == 4 {
				cut = i + 1
				break
			}
		}
	}

	fracPart = fracPart[:cut]
	return sign + intPart + "." + fracPart
}

type RangeAlertLines struct {
	Current string
	Lower   string
	Upper   string
}

func FormatRangeAlertLines(task *models.StrategyTask, tickLower, tickUpper, currentTick int) RangeAlertLines {
	display := BuildPriceRangeDisplay(task, tickLower, tickUpper, currentTick)
	currentLine := fmt.Sprintf("当前 Tick: %d", currentTick)
	lowerLine := fmt.Sprintf("区间下界: %d", tickLower)
	upperLine := fmt.Sprintf("区间上界: %d", tickUpper)

	if display.HasCurrent {
		currentLine = fmt.Sprintf("当前价格：1 %s ≈ %s %s", display.BaseSymbol, FormatPriceValue(display.Current), display.QuoteSymbol)
	}
	if display.HasRange {
		lowerLine = fmt.Sprintf("区间下界：%s %s", FormatPriceValue(display.Lower), display.QuoteSymbol)
		upperLine = fmt.Sprintf("区间上界：%s %s", FormatPriceValue(display.Upper), display.QuoteSymbol)
	}

	return RangeAlertLines{
		Current: currentLine,
		Lower:   lowerLine,
		Upper:   upperLine,
	}
}
