package pricing

import (
	"TgLpBot/base/blockchain"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// PancakeSwap V3 WBNB/USDT pool (0.01% fee).
const pancakeV3WBNBUSDTPool = "0x172fcD41E0913e95784454622d1c3724f546f849"

// GetBNBPriceUSDT reads BNB price from PancakeSwap V3 WBNB/USDT pool.
func GetBNBPriceUSDT() float64 {
	if blockchain.Client == nil {
		log.Printf("[Pricing] blockchain client not initialized; fallback BNB price")
		return 700.0
	}

	poolAddr := common.HexToAddress(pancakeV3WBNBUSDTPool)
	sqrtPriceX96, _, err := blockchain.GetV3PoolSlot0(poolAddr)
	if err != nil {
		log.Printf("[Pricing] fetch BNB price failed: %v; fallback", err)
		return 700.0
	}

	q96 := new(big.Int).Lsh(big.NewInt(1), 96)
	p := new(big.Float).SetInt(sqrtPriceX96)
	q := new(big.Float).SetInt(q96)
	p.Quo(p, q)
	p.Mul(p, p)
	priceWBNBperUSDT, _ := p.Float64()

	priceF64 := 0.0
	if priceWBNBperUSDT > 0 {
		priceF64 = 1.0 / priceWBNBperUSDT
	}
	if priceF64 <= 0 || priceF64 > 100000 {
		log.Printf("[Pricing] BNB price out of range: %.2f (raw %.10f); fallback", priceF64, priceWBNBperUSDT)
		return 700.0
	}

	return priceF64
}
