package main

import (
	"fmt"
	"strconv"
)

// 模型结构
type PoolData struct {
	Token0              TokenData   `json:"token0"`
	Token1              TokenData   `json:"token1"`
	FeeTier             string      `json:"feeTier"` // 500 = 0.05%
	TotalValueLockedUSD string      `json:"totalValueLockedUSD"`
	PoolDayData         []DailyData `json:"poolDayData"`
}

type TokenData struct {
	Symbol string `json:"symbol"`
}

type DailyData struct {
	VolumeUSD string `json:"volumeUSD"`
	FeesUSD   string `json:"feesUSD"`
}

func main() {
	fmt.Println("🚀 开始抓取 Uniswap V3/V4 池子数据 (数据结构演示)...")
	fmt.Println("📊 ⚠️ 注：由于公共 Graph 端点现需 API Key，本 Demo 使用【模拟数据】展示我们将获取到的字段和计算逻辑")
	fmt.Println("=========================================================================================================")
	fmt.Printf("%-20s | %-10s | %-13s | %-13s | %-13s | %-8s | %-6s\n", "Pool Pair", "Fee Tier", "TVL ($)", "24h Vol ($)", "24h Fees ($)", "APR (%)", "Type")
	fmt.Println("---------------------------------------------------------------------------------------------------------")

	// 模拟抓取到的数据（接入 API Key 后就是这些真实数据）
	simulatedPools := []PoolData{
		{
			Token0: TokenData{Symbol: "USDT"}, Token1: TokenData{Symbol: "WBNB"},
			FeeTier: "500", TotalValueLockedUSD: "150000000",
			PoolDayData: []DailyData{{VolumeUSD: "45000000", FeesUSD: "22500"}}, // 0.05% * 45M (近似)
		},
		{
			Token0: TokenData{Symbol: "ETH"}, Token1: TokenData{Symbol: "USDC"},
			FeeTier: "3000", TotalValueLockedUSD: "280000000",
			PoolDayData: []DailyData{{VolumeUSD: "120000000", FeesUSD: "360000"}}, // 0.3%
		},
		{
			Token0: TokenData{Symbol: "CAKE"}, Token1: TokenData{Symbol: "WBNB"},
			FeeTier: "2500", TotalValueLockedUSD: "5000000",
			PoolDayData: []DailyData{{VolumeUSD: "8000000", FeesUSD: "20000"}}, // 0.25%
		},
		{
			Token0: TokenData{Symbol: "PEPE"}, Token1: TokenData{Symbol: "WETH"},
			FeeTier: "10000", TotalValueLockedUSD: "2000000",
			PoolDayData: []DailyData{{VolumeUSD: "15000000", FeesUSD: "150000"}}, // 1% (金狗！)
		},
		{
			Token0: TokenData{Symbol: "HOOK"}, Token1: TokenData{Symbol: "WBNB"},
			FeeTier: "0", TotalValueLockedUSD: "120000",
			PoolDayData: []DailyData{{VolumeUSD: "800000", FeesUSD: "4000"}}, // V4 模拟
		},
	}

	// 3. 处理并打印数据
	for _, p := range simulatedPools {
		symbol := fmt.Sprintf("%s/%s", p.Token0.Symbol, p.Token1.Symbol)
		poolType := "V3"

		feeTierInt, _ := strconv.Atoi(p.FeeTier)
		feeParams := float64(feeTierInt) / 10000.0
		feePercent := fmt.Sprintf("%.2f%%", feeParams*100)

		// 模拟 V4 标识
		if p.FeeTier == "0" {
			poolType = "V4"
			feePercent = "Dynamic" // V4 动态费率
		}

		tvl, _ := strconv.ParseFloat(p.TotalValueLockedUSD, 64)

		// 获取 24h 数据
		vol24h := 0.0
		fees24h := 0.0
		if len(p.PoolDayData) > 0 {
			vol24h, _ = strconv.ParseFloat(p.PoolDayData[0].VolumeUSD, 64)

			if p.PoolDayData[0].FeesUSD != "" {
				fees24h, _ = strconv.ParseFloat(p.PoolDayData[0].FeesUSD, 64)
			} else {
				fees24h = vol24h * feeParams
			}
		}

		// 计算 APR: (24h Fees / TVL) * 365
		apr := 0.0
		if tvl > 0 {
			apr = (fees24h / tvl) * 365.0 * 100.0
		}

		// 打印一行结果
		fmt.Printf("%-20s | %-10s | $%-12.0f | $%-12.0f | $%-12.0f | %6.2f%% | %s\n",
			limitStr(symbol, 20),
			feePercent,
			tvl,
			vol24h,
			fees24h,
			apr,
			poolType,
		)
	}

	fmt.Println("=========================================================================================================")
	fmt.Println("\n🔎 [图表分析] 为什么 The Graph 数据源更优？")
	fmt.Println("1. 费用维度 (Fees): PEPE/WETH(1%) 和 USDT/WBNB(0.05%) 即使交易量相同，实际收入差 20 倍！Gecko 无法区分这个，Graph 可以。")
	fmt.Println("2. APR 计算: 只有结合了 TVL 和 真实Fees，才能算出准确的 APR (最后一列)。")
	fmt.Println("3. 筛选能力: 我们可以直接用 `orderBy: feesUSD` 找出全网实际最赚钱的池子，而不仅仅是最热闹的。")
	fmt.Println("\n🦄 [Uniswap V4 支持方案]")
	fmt.Println("针对 V4，我们将构建独立的监听服务：")
	fmt.Println("  - 监听 PoolManager 合约的 Initialize 和 Swap 事件")
	fmt.Println("  - 实时聚合 Volume 和 Fees (V4 允许 Hook 提取费用，需要单独计算)")
	fmt.Println("  - 最终将数据标准化为上述格式，供策略引擎消费。")
}

func limitStr(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
