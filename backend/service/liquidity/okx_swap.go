package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/service/exchange"
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func normalizeOkxSwapGasMultiplier(v float64) float64 {
	if v <= 0 {
		return 1
	}
	if v > 10 {
		return 10
	}
	return v
}

// okxSwapGasLimit picks a safe gasLimit for OKX swap tx:
// - use node EstimateGas when possible
// - take max(estimate, okxSuggested)
// - apply a safety multiplier (default 1.30) and min/max bounds if configured
func okxSwapGasLimit(from common.Address, to common.Address, value *big.Int, data []byte, okxSuggested uint64) (uint64, error) {
	if blockchain.Client == nil {
		return 0, fmt.Errorf("blockchain client not initialized")
	}
	if value == nil {
		value = big.NewInt(0)
	}

	msg := ethereum.CallMsg{
		From:  from,
		To:    &to,
		Value: value,
		Data:  data,
	}

	estimated, err := blockchain.Client.EstimateGas(context.Background(), msg)
	if err != nil {
		if okxSuggested == 0 {
			return 0, fmt.Errorf("estimate gas failed: %w", err)
		}
		log.Printf("[Liquidity] Warning: OKX swap EstimateGas failed, fallback to OKX gas=%d: %v", okxSuggested, err)
		estimated = okxSuggested
	}

	base := estimated
	if okxSuggested > base {
		base = okxSuggested
	}

	mult := 1.30
	minLimit := uint64(0)
	maxLimit := uint64(0)
	if config.AppConfig != nil {
		if config.AppConfig.OKXSwapGasLimitMultiplier > 0 {
			mult = config.AppConfig.OKXSwapGasLimitMultiplier
		}
		minLimit = config.AppConfig.OKXSwapGasLimitMin
		maxLimit = config.AppConfig.OKXSwapGasLimitMax
	}
	mult = normalizeOkxSwapGasMultiplier(mult)

	// gas values are small (< block limit), float64 is safe here.
	withMult := uint64(float64(base) * mult)
	gasLimit := withMult
	if gasLimit < base {
		gasLimit = base
	}
	if minLimit > 0 && gasLimit < minLimit {
		gasLimit = minLimit
	}
	if maxLimit > 0 && gasLimit > maxLimit {
		gasLimit = maxLimit
	}
	return gasLimit, nil
}

// swapExactInViaOKX executes a swap transaction returned by OKX DEX /swap API from the user's wallet.
// It returns the balance delta of tokenOut observed on the wallet.
func (s *LiquidityService) swapExactInViaOKX(
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*big.Int, error) {
	if config.AppConfig == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	if blockchain.Client == nil || blockchain.ChainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return big.NewInt(0), nil
	}
	if tokenIn == tokenOut {
		return new(big.Int).Set(amountIn), nil
	}

	if s.okxService == nil {
		s.okxService = exchange.NewOKXDexService()
	}

	outBefore, _ := blockchain.GetTokenBalance(tokenOut, walletAddr)
	if outBefore == nil {
		outBefore = big.NewInt(0)
	}

	swapReq := exchange.SwapRequest{
		ChainID:           fmt.Sprintf("%d", config.AppConfig.BSCChainID),
		FromTokenAddress:  tokenIn.Hex(),
		ToTokenAddress:    tokenOut.Hex(),
		Amount:            amountIn.String(),
		Slippage:          s.okxSlippageDecimal(slippagePercent),
		UserWalletAddress: walletAddr.Hex(),
	}

	swapResp, err := s.okxService.GetSwapData(swapReq)
	if err != nil {
		return nil, err
	}
	if swapResp == nil || len(swapResp.Data) == 0 {
		return nil, fmt.Errorf("OKX swap response empty")
	}

	expectedOut := strings.TrimSpace(swapResp.Data[0].RouterResult.ToTokenAmount)
	if expectedOut == "" {
		expectedOut = "unknown"
	}
	estGas := strings.TrimSpace(swapResp.Data[0].Tx.Gas)
	if estGas != "" {
		log.Printf("[Liquidity] OKX swap preview: %s -> %s amountIn=%s expectedOut=%s txGas=%s slippage=%.4f%%",
			tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), expectedOut, estGas, slippagePercent)
	} else {
		log.Printf("[Liquidity] OKX swap preview: %s -> %s amountIn=%s expectedOut=%s slippage=%.4f%%",
			tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), expectedOut, slippagePercent)
	}

	txObj := swapResp.Data[0].Tx
	if !common.IsHexAddress(txObj.To) {
		return nil, fmt.Errorf("OKX tx.to invalid: %s", txObj.To)
	}
	to := common.HexToAddress(txObj.To)
	data := common.FromHex(txObj.Data)
	if len(data) == 0 {
		return nil, fmt.Errorf("OKX tx.data empty")
	}

	value := new(big.Int)
	if strings.TrimSpace(txObj.Value) != "" {
		if v, ok := new(big.Int).SetString(strings.TrimSpace(txObj.Value), 10); ok {
			value = v
		} else if v, ok := new(big.Int).SetString(strings.TrimPrefix(strings.TrimSpace(txObj.Value), "0x"), 16); ok {
			value = v
		}
	}
	if value.Sign() != 0 {
		return nil, fmt.Errorf("OKX swap requires native value; not supported")
	}

	var okxGasLimit uint64
	if strings.TrimSpace(txObj.Gas) != "" {
		if g, ok := new(big.Int).SetString(strings.TrimSpace(txObj.Gas), 10); ok && g.IsUint64() {
			okxGasLimit = g.Uint64()
		}
	}

	swapTx := blockchain.OkxSwapTx{To: to, Value: value, Data: data}
	_ = ValidateOkxSmartSwapTx("swap", swapTx)
	if err := EnforceOkxSwapRouter("swap", swapTx); err != nil {
		return nil, err
	}

	// 获取 OKX TokenApprove 合约地址
	chainID := fmt.Sprintf("%d", config.AppConfig.BSCChainID)
	approveSpender, err := s.okxService.GetApproveSpender(chainID, tokenIn.Hex())
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get OKX approve spender, using router as fallback: %v", err)
		approveSpender = to.Hex() // 降级到使用 router
	}
	approveAddr := common.HexToAddress(approveSpender)

	log.Printf("[Liquidity] OKX swap: %s -> %s amount=%s router=%s approveTarget=%s",
		tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), to.Hex(), approveAddr.Hex())

	// Approve TokenApprove 合约 to spend tokenIn
	if err := s.approveToken(privateKey, walletAddr, tokenIn, approveAddr, amountIn, TxOptions{}); err != nil {
		return nil, fmt.Errorf("approve TokenApprove contract failed: %w", err)
	}

	nonce, err := blockchain.GetNonce(walletAddr)
	if err != nil {
		return nil, err
	}
	gasPrice, err := blockchain.GetGasPrice()
	if err != nil {
		return nil, err
	}

	gasLimit, err := okxSwapGasLimit(walletAddr, to, value, data, okxGasLimit)
	if err != nil {
		return nil, err
	}
	log.Printf("[Liquidity] OKX swap gasLimit: okx=%d final=%d", okxGasLimit, gasLimit)

	rawTx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, data)
	signed, err := types.SignTx(rawTx, types.NewEIP155Signer(blockchain.ChainID), privateKey)
	if err != nil {
		return nil, err
	}
	if _, err := blockchain.SendTransaction(signed); err != nil {
		return nil, err
	}
	txHash := signed.Hash().Hex()
	receipt, err := s.waitMined(signed)
	if err != nil {
		return nil, err
	}

	// Prefer receipt logs over balance reads to avoid "RPC lag / load balancer" stale reads.
	if d := ReceiptTokenTransferDelta(receipt, tokenOut, walletAddr); d != nil && d.Sign() > 0 {
		return d, nil
	}

	outAfter, _ := blockchain.GetTokenBalance(tokenOut, walletAddr)
	if outAfter == nil {
		outAfter = big.NewInt(0)
	}
	delta := new(big.Int).Sub(outAfter, outBefore)
	if delta.Sign() <= 0 {
		minWanted := new(big.Int).Add(outBefore, big.NewInt(1))
		if synced, werr := s.waitTokenBalanceAtLeast(tokenOut, walletAddr, minWanted, "OKX swap out"); werr == nil && synced != nil {
			outAfter = synced
			delta = new(big.Int).Sub(outAfter, outBefore)
		}
	}
	if delta.Sign() < 0 {
		delta = big.NewInt(0)
	}
	if delta.Sign() == 0 {
		expectedOut := strings.TrimSpace(swapResp.Data[0].RouterResult.ToTokenAmount)
		if expectedOut == "" {
			expectedOut = "unknown"
		}
		log.Printf("[Liquidity] Warning: OKX swap mined but tokenOut delta is 0 (tx=%s tokenOut=%s expected=%s outBefore=%s outAfter=%s)",
			txHash, tokenOut.Hex(), expectedOut, outBefore.String(), outAfter.String())
	}
	return delta, nil
}
