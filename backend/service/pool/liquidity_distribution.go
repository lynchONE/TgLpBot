package pool

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/service/chainexec"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/sync/errgroup"
)

const (
	// liquidityDistMinRadius / MaxRadius 限制 bin 数量，避免一次过多 RPC 拖垮节点。
	liquidityDistMinRadius  = 5
	liquidityDistMaxRadius  = 60
	liquidityDistMaxConcur  = 10
	liquidityDistRPCTimeout = 8 * time.Second
)

// LiquidityBin 表示某个 tick 区间内的活跃流动性。Liquidity 是 *big.Int 的十进制字符串
// 表示，前端拿到后用大数库或纯 string 比较即可，不丢精度。
type LiquidityBin struct {
	Index     int    `json:"index"`
	TickLower int    `json:"tick_lower"`
	TickUpper int    `json:"tick_upper"`
	Liquidity string `json:"liquidity"`
	IsActive  bool   `json:"is_active"`
}

// LiquidityProfile 是返回给前端的整体结构。
type LiquidityProfile struct {
	Chain           string         `json:"chain"`
	Protocol        string         `json:"protocol"`
	Address         string         `json:"address"`
	CurrentTick     int            `json:"current_tick"`
	TickSpacing     int            `json:"tick_spacing"`
	ActiveLiquidity string         `json:"active_liquidity"`
	SqrtPriceX96    string         `json:"sqrt_price_x96"`
	Bins            []LiquidityBin `json:"bins"`
	GeneratedAt     int64          `json:"generated_at"`
}

// 行内 ABI：V3 ticks(int24) 与 V4 StateView.getTickInfo(bytes32, int24)。
// 两者前两个返回字段都是 (uint128 liquidityGross, int128 liquidityNet)，
// 这里只解到这两个就够了，省去其他 fee growth 字段的字节。
const v3TicksLiquidityABI = `[
  {
    "inputs": [{"internalType":"int24","name":"tick","type":"int24"}],
    "name": "ticks",
    "outputs": [
      {"internalType":"uint128","name":"liquidityGross","type":"uint128"},
      {"internalType":"int128","name":"liquidityNet","type":"int128"},
      {"internalType":"uint256","name":"feeGrowthOutside0X128","type":"uint256"},
      {"internalType":"uint256","name":"feeGrowthOutside1X128","type":"uint256"},
      {"internalType":"int56","name":"tickCumulativeOutside","type":"int56"},
      {"internalType":"uint160","name":"secondsPerLiquidityOutsideX128","type":"uint160"},
      {"internalType":"uint32","name":"secondsOutside","type":"uint32"},
      {"internalType":"bool","name":"initialized","type":"bool"}
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

const v4StateViewTickInfoABI = `[
  {
    "inputs": [
      {"internalType":"bytes32","name":"poolId","type":"bytes32"},
      {"internalType":"int24","name":"tick","type":"int24"}
    ],
    "name": "getTickInfo",
    "outputs": [
      {"internalType":"uint128","name":"liquidityGross","type":"uint128"},
      {"internalType":"int128","name":"liquidityNet","type":"int128"},
      {"internalType":"uint256","name":"feeGrowthOutside0X128","type":"uint256"},
      {"internalType":"uint256","name":"feeGrowthOutside1X128","type":"uint256"}
    ],
    "stateMutability": "view",
    "type": "function"
  }
]`

var (
	v3TicksABIOnce sync.Once
	v3TicksABI     abi.ABI
	v3TicksABIErr  error

	v4TickInfoABIOnce sync.Once
	v4TickInfoABI     abi.ABI
	v4TickInfoABIErr  error
)

func parseV3TicksABI() (abi.ABI, error) {
	v3TicksABIOnce.Do(func() {
		v3TicksABI, v3TicksABIErr = abi.JSON(strings.NewReader(v3TicksLiquidityABI))
	})
	return v3TicksABI, v3TicksABIErr
}

func parseV4TickInfoABI() (abi.ABI, error) {
	v4TickInfoABIOnce.Do(func() {
		v4TickInfoABI, v4TickInfoABIErr = abi.JSON(strings.NewReader(v4StateViewTickInfoABI))
	})
	return v4TickInfoABI, v4TickInfoABIErr
}

// alignDownToSpacing 把 tick 向下对齐到 tickSpacing 的倍数。
func alignDownToSpacing(tick, spacing int) int {
	if spacing <= 0 {
		return tick
	}
	rem := tick % spacing
	if rem == 0 {
		return tick
	}
	if tick < 0 {
		return tick - rem - spacing
	}
	return tick - rem
}

// GetLiquidityDistribution 返回当前 tick 附近 [-radius, +radius] 共 2*radius+1 个 bin
// 的活跃流动性分布。protocol 取值 "v3" 或 "v4"。address 在 V3 是池子地址，V4 是 poolId。
func GetLiquidityDistribution(ctx context.Context, chain, protocol, address string, radius int) (*LiquidityProfile, error) {
	chain = config.NormalizeChain(chain)
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("address required")
	}
	if radius < liquidityDistMinRadius {
		radius = liquidityDistMinRadius
	}
	if radius > liquidityDistMaxRadius {
		radius = liquidityDistMaxRadius
	}

	exec, err := chainexec.GetEVM(chain)
	if err != nil {
		return nil, fmt.Errorf("get chain executor: %w", err)
	}
	client := exec.Client()
	if client == nil {
		return nil, fmt.Errorf("chain client not available")
	}

	switch protocol {
	case "v3":
		return getV3LiquidityDistribution(ctx, exec, client, address, radius)
	case "v4":
		return getV4LiquidityDistribution(ctx, exec, client, address, radius)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

func getV3LiquidityDistribution(ctx context.Context, exec chainexec.EVMExecutor, client *ethclient.Client, address string, radius int) (*LiquidityProfile, error) {
	if !common.IsHexAddress(address) {
		return nil, fmt.Errorf("invalid v3 pool address: %s", address)
	}
	poolAddr := common.HexToAddress(address)

	sqrtPriceX96, currentTick, err := blockchain.GetV3PoolSlot0WithClient(client, poolAddr)
	if err != nil {
		return nil, fmt.Errorf("v3 slot0: %w", err)
	}
	activeLiq, err := blockchain.GetV3PoolLiquidityWithClient(client, poolAddr)
	if err != nil {
		return nil, fmt.Errorf("v3 liquidity: %w", err)
	}
	tickSpacing, err := getV3PoolTickSpacing(ctx, client, poolAddr)
	if err != nil {
		return nil, fmt.Errorf("v3 tick spacing: %w", err)
	}

	parsed, err := parseV3TicksABI()
	if err != nil {
		return nil, fmt.Errorf("parse v3 ticks ABI: %w", err)
	}

	tickLowerAnchor := alignDownToSpacing(currentTick, tickSpacing)
	netByBoundary, err := fetchTickLiquidityNets(ctx, radius, tickSpacing, tickLowerAnchor, func(ctx context.Context, tick int) (*big.Int, error) {
		return callV3TickLiquidityNet(ctx, client, parsed, poolAddr, tick)
	})
	if err != nil {
		return nil, err
	}

	bins := buildBinsFromBoundaryNets(activeLiq, currentTick, tickSpacing, tickLowerAnchor, radius, netByBoundary)
	return &LiquidityProfile{
		Chain:           exec.Chain(),
		Protocol:        "v3",
		Address:         strings.ToLower(poolAddr.Hex()),
		CurrentTick:     currentTick,
		TickSpacing:     tickSpacing,
		ActiveLiquidity: bigIntString(activeLiq),
		SqrtPriceX96:    bigIntString(sqrtPriceX96),
		Bins:            bins,
		GeneratedAt:     time.Now().Unix(),
	}, nil
}

func getV4LiquidityDistribution(ctx context.Context, exec chainexec.EVMExecutor, client *ethclient.Client, poolID string, radius int) (*LiquidityProfile, error) {
	cc := exec.Config()
	stateViewHex := strings.TrimSpace(cc.UniswapV4StateViewAddress)
	if stateViewHex == "" {
		return nil, fmt.Errorf("v4 stateView address not configured for chain %s", exec.Chain())
	}
	if !common.IsHexAddress(stateViewHex) {
		return nil, fmt.Errorf("invalid v4 stateView address: %s", stateViewHex)
	}
	poolManagerHex := strings.TrimSpace(cc.UniswapV4PoolManagerAddress)
	if poolManagerHex == "" {
		return nil, fmt.Errorf("v4 poolManager address not configured for chain %s", exec.Chain())
	}
	stateView := common.HexToAddress(stateViewHex)
	poolManager := common.HexToAddress(poolManagerHex)

	sqrtPriceX96, currentTick, err := blockchain.GetUniswapV4PoolSlot0ViaStateView(stateView, poolManager, poolID)
	if err != nil {
		return nil, fmt.Errorf("v4 slot0: %w", err)
	}
	activeLiq, err := blockchain.GetUniswapV4PoolLiquidityViaStateView(stateView, poolManager, poolID)
	if err != nil {
		return nil, fmt.Errorf("v4 liquidity: %w", err)
	}
	tickSpacing, err := blockchain.GetUniswapV4PoolTickSpacing(poolManager, poolID)
	if err != nil || tickSpacing <= 0 {
		// Fallback: PositionManager.poolKeys(bytes25) — 很多部署只在 PositionManager 存 PoolKey 映射。
		pmHex := strings.TrimSpace(cc.UniswapV4PositionManagerAddress)
		if pmHex != "" && common.IsHexAddress(pmHex) {
			if _, _, _, ts, _, pmErr := blockchain.GetUniswapV4PoolKeyFromPositionManager(common.HexToAddress(pmHex), poolID); pmErr == nil && ts > 0 {
				tickSpacing = ts
				err = nil
			} else if pmErr != nil {
				err = fmt.Errorf("poolManager+positionManager both failed: %w", pmErr)
			}
		}
		if err != nil || tickSpacing <= 0 {
			return nil, fmt.Errorf("v4 tick spacing: %w", err)
		}
	}

	parsed, err := parseV4TickInfoABI()
	if err != nil {
		return nil, fmt.Errorf("parse v4 tick info ABI: %w", err)
	}
	poolIDHash, err := normalizePoolIDForV4(poolID)
	if err != nil {
		return nil, fmt.Errorf("normalize poolId: %w", err)
	}

	tickLowerAnchor := alignDownToSpacing(currentTick, tickSpacing)
	netByBoundary, err := fetchTickLiquidityNets(ctx, radius, tickSpacing, tickLowerAnchor, func(ctx context.Context, tick int) (*big.Int, error) {
		return callV4TickLiquidityNet(ctx, client, parsed, stateView, poolIDHash, tick)
	})
	if err != nil {
		return nil, err
	}

	bins := buildBinsFromBoundaryNets(activeLiq, currentTick, tickSpacing, tickLowerAnchor, radius, netByBoundary)
	return &LiquidityProfile{
		Chain:           exec.Chain(),
		Protocol:        "v4",
		Address:         strings.ToLower(poolID),
		CurrentTick:     currentTick,
		TickSpacing:     tickSpacing,
		ActiveLiquidity: bigIntString(activeLiq),
		SqrtPriceX96:    bigIntString(sqrtPriceX96),
		Bins:            bins,
		GeneratedAt:     time.Now().Unix(),
	}, nil
}

// fetchTickLiquidityNets 并发拉取 [tickLowerAnchor - radius*spacing, tickLowerAnchor + (radius+1)*spacing]
// 这一组 tick 边界点的 liquidityNet。返回 map[tick]*big.Int。
func fetchTickLiquidityNets(
	ctx context.Context,
	radius int,
	spacing int,
	tickLowerAnchor int,
	fetch func(ctx context.Context, tick int) (*big.Int, error),
) (map[int]*big.Int, error) {
	boundaries := make([]int, 0, 2*radius+2)
	for i := -radius; i <= radius+1; i++ {
		boundaries = append(boundaries, tickLowerAnchor+i*spacing)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(liquidityDistMaxConcur)

	var mu sync.Mutex
	result := make(map[int]*big.Int, len(boundaries))

	for _, tick := range boundaries {
		tick := tick
		g.Go(func() error {
			callCtx, cancel := context.WithTimeout(gctx, liquidityDistRPCTimeout)
			defer cancel()
			net, err := fetch(callCtx, tick)
			if err != nil {
				// 单个 tick RPC 失败时记 0，不让整体接口失败 — 视觉上仅显示该 bin 与相邻 bin 相同的活跃流动性。
				net = big.NewInt(0)
			}
			mu.Lock()
			result[tick] = net
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}

// buildBinsFromBoundaryNets 从 currentTick 出发左右扫描，逐 bin 累加 liquidityNet 还原 active liquidity。
// 算法依据 Uniswap V3/V4 white paper：穿过一个 initialized tick 时 liquidity ± liquidityNet。
//   - 向上越过 tick：liquidity += liquidityNet(tick)
//   - 向下越过 tick：liquidity -= liquidityNet(tick)
func buildBinsFromBoundaryNets(activeLiq *big.Int, currentTick, spacing, tickLowerAnchor, radius int, nets map[int]*big.Int) []LiquidityBin {
	bins := make([]LiquidityBin, 0, 2*radius+1)
	if activeLiq == nil {
		activeLiq = big.NewInt(0)
	}

	type binState struct {
		index     int
		tickLower int
		tickUpper int
		liquidity *big.Int
	}
	states := make([]binState, 0, 2*radius+1)
	for i := -radius; i <= radius; i++ {
		lower := tickLowerAnchor + i*spacing
		states = append(states, binState{
			index:     i,
			tickLower: lower,
			tickUpper: lower + spacing,
			liquidity: new(big.Int),
		})
	}

	// 找到包含 currentTick 的 bin（index 偏移）
	currentIdx := radius
	current := new(big.Int).Set(activeLiq)
	states[currentIdx].liquidity.Set(current)

	// 向上扫：穿过 states[i].tickUpper 后，新 bin liquidity = current + net(tickUpper)
	for i := currentIdx; i < len(states)-1; i++ {
		nextTick := states[i].tickUpper
		net := nets[nextTick]
		if net == nil {
			net = big.NewInt(0)
		}
		current = new(big.Int).Add(current, net)
		states[i+1].liquidity.Set(maxZero(current))
	}

	// 向下扫：穿过 states[i].tickLower 后，下一 bin liquidity = current - net(tickLower)
	current = new(big.Int).Set(activeLiq)
	for i := currentIdx; i > 0; i-- {
		boundary := states[i].tickLower
		net := nets[boundary]
		if net == nil {
			net = big.NewInt(0)
		}
		current = new(big.Int).Sub(current, net)
		states[i-1].liquidity.Set(maxZero(current))
	}

	for _, st := range states {
		bins = append(bins, LiquidityBin{
			Index:     st.index,
			TickLower: st.tickLower,
			TickUpper: st.tickUpper,
			Liquidity: bigIntString(st.liquidity),
			IsActive:  currentTick >= st.tickLower && currentTick < st.tickUpper,
		})
	}
	return bins
}

func maxZero(v *big.Int) *big.Int {
	if v == nil || v.Sign() < 0 {
		return big.NewInt(0)
	}
	return v
}

func bigIntString(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

// getV3PoolTickSpacing 通过最小 ABI 调用 tickSpacing()。V3 池子可能没有此函数（早期 Uniswap V2 仿盘
// 不会用 V3 接口），这里失败时按 fee tier 推断或回退到 60。
func getV3PoolTickSpacing(ctx context.Context, client *ethclient.Client, poolAddr common.Address) (int, error) {
	const tickSpacingABI = `[{"inputs":[],"name":"tickSpacing","outputs":[{"internalType":"int24","name":"","type":"int24"}],"stateMutability":"view","type":"function"}]`
	parsed, err := abi.JSON(strings.NewReader(tickSpacingABI))
	if err != nil {
		return 0, err
	}
	data, err := parsed.Pack("tickSpacing")
	if err != nil {
		return 0, err
	}
	callCtx, cancel := context.WithTimeout(ctx, liquidityDistRPCTimeout)
	defer cancel()
	raw, err := client.CallContract(callCtx, ethereum.CallMsg{To: &poolAddr, Data: data}, nil)
	if err == nil {
		out, unpackErr := parsed.Unpack("tickSpacing", raw)
		if unpackErr == nil && len(out) > 0 {
			if v, ok := out[0].(*big.Int); ok && v != nil && v.Sign() > 0 {
				return int(v.Int64()), nil
			}
		}
	}
	// fallback：按 fee 推断
	fee, feeErr := blockchain.GetV3PoolFeeWithClient(client, poolAddr)
	if feeErr != nil {
		return 60, nil
	}
	return tickSpacingFromFee(int(fee)), nil
}

func tickSpacingFromFee(fee int) int {
	switch fee {
	case 100:
		return 1
	case 500:
		return 10
	case 2500:
		return 50
	case 3000:
		return 60
	case 10000:
		return 200
	}
	return 60
}

func callV3TickLiquidityNet(ctx context.Context, client *ethclient.Client, parsed abi.ABI, pool common.Address, tick int) (*big.Int, error) {
	data, err := parsed.Pack("ticks", big.NewInt(int64(tick)))
	if err != nil {
		return nil, err
	}
	raw, err := client.CallContract(ctx, ethereum.CallMsg{To: &pool, Data: data}, nil)
	if err != nil {
		// uninitialized tick 返回 revert 是常见情况，按 0 处理
		if strings.Contains(strings.ToLower(err.Error()), "revert") {
			return big.NewInt(0), nil
		}
		return nil, err
	}
	out, err := parsed.Unpack("ticks", raw)
	if err != nil || len(out) < 2 {
		return big.NewInt(0), nil
	}
	v, _ := out[1].(*big.Int)
	if v == nil {
		return big.NewInt(0), nil
	}
	return v, nil
}

func callV4TickLiquidityNet(ctx context.Context, client *ethclient.Client, parsed abi.ABI, stateView common.Address, poolID common.Hash, tick int) (*big.Int, error) {
	data, err := parsed.Pack("getTickInfo", poolID, big.NewInt(int64(tick)))
	if err != nil {
		return nil, err
	}
	raw, err := client.CallContract(ctx, ethereum.CallMsg{To: &stateView, Data: data}, nil)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "revert") {
			return big.NewInt(0), nil
		}
		return nil, err
	}
	out, err := parsed.Unpack("getTickInfo", raw)
	if err != nil || len(out) < 2 {
		return big.NewInt(0), nil
	}
	v, _ := out[1].(*big.Int)
	if v == nil {
		return big.NewInt(0), nil
	}
	return v, nil
}

// normalizePoolIDForV4 把传入的 poolId（可能带 0x 前缀，或 64 位 hex 字符串）规范化为 common.Hash。
func normalizePoolIDForV4(poolID string) (common.Hash, error) {
	s := strings.TrimSpace(poolID)
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	if len(s) != 64 {
		return common.Hash{}, fmt.Errorf("v4 poolId must be 32 bytes hex, got %d hex chars", len(s))
	}
	b := common.HexToHash("0x" + s)
	return b, nil
}
