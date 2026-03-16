package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/chainexec"
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// BuildOKXSwapParams exposes the existing zap swap construction logic for create-pool flows.
func (s *LiquidityService) BuildOKXSwapParams(
	cc config.ChainConfig,
	executorAddr common.Address,
	tokenIn, tokenOut common.Address,
	amountIn *big.Int,
	slippageTolerance float64,
) (*blockchain.SwapParamsSimple, *big.Int, error) {
	return s.prepareOKXSwapParams(cc, executorAddr, tokenIn, tokenOut, amountIn, slippageTolerance)
}

// CalculateOptimalSwapForRange estimates the swap direction/amount needed for a target range.
func (s *LiquidityService) CalculateOptimalSwapForRange(
	sqrtPriceX96 *big.Int,
	currentTick, tickLower, tickUpper int,
	amount0In, amount1In *big.Int,
) (bool, *big.Int, error) {
	return s.calculateOptimalSwapPure(sqrtPriceX96, currentTick, tickLower, tickUpper, amount0In, amount1In)
}

// EnterV3FromToken reuses the existing zap-based single-token entry flow for create-pool.
func (s *LiquidityService) EnterV3FromToken(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	amountIn *big.Int,
	task *models.StrategyTask,
	opts TxOptions,
) (*EnterResult, error) {
	return s.enterV3FromToken(exec, wallet, privateKey, walletAddr, tokenIn, amountIn, task, opts)
}

// EnterV4FromToken reuses the existing zap-based single-token entry flow for create-pool.
func (s *LiquidityService) EnterV4FromToken(
	exec chainexec.EVMExecutor,
	wallet *models.Wallet,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	amountIn *big.Int,
	task *models.StrategyTask,
	opts TxOptions,
) (*EnterResult, error) {
	return s.enterV4FromToken(exec, wallet, privateKey, walletAddr, tokenIn, amountIn, task, opts)
}
