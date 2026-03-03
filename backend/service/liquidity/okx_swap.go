package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/service/chainexec"
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
	"github.com/ethereum/go-ethereum/ethclient"
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
func okxSwapGasLimit(client *ethclient.Client, from common.Address, to common.Address, value *big.Int, data []byte, okxSuggested uint64) (uint64, error) {
	if client == nil {
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

	estimated, err := client.EstimateGas(context.Background(), msg)
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

type okxSwapExecutionResult struct {
	TxHash   string
	Receipt  *types.Receipt
	DeltaOut *big.Int
}

// executeOKXSwapExactIn executes a swap transaction returned by OKX DEX /swap API from the user's wallet.
// It returns txHash + receipt + tokenOut balance delta (best-effort).
func (s *LiquidityService) executeOKXSwapExactIn(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*okxSwapExecutionResult, error) {
	if exec == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	cc := exec.Config()
	client := exec.Client()
	chainID := exec.ChainID()

	if client == nil || chainID == nil {
		return nil, fmt.Errorf("blockchain client not initialized")
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return &okxSwapExecutionResult{DeltaOut: big.NewInt(0)}, nil
	}
	if tokenIn == tokenOut {
		return &okxSwapExecutionResult{DeltaOut: new(big.Int).Set(amountIn)}, nil
	}

	if s.okxService == nil {
		s.okxService = exchange.NewOKXDexService()
	}

	outBefore, _ := blockchain.GetTokenBalanceWithClient(client, tokenOut, walletAddr)
	if outBefore == nil {
		outBefore = big.NewInt(0)
	}

	swapReq := exchange.SwapRequest{
		ChainID:           fmt.Sprintf("%d", cc.ChainID),
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
		log.Printf("[Liquidity] OKX swap preview: chain=%s %s -> %s amountIn=%s expectedOut=%s txGas=%s slippage=%.4f%%",
			exec.Chain(), tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), expectedOut, estGas, slippagePercent)
	} else {
		log.Printf("[Liquidity] OKX swap preview: chain=%s %s -> %s amountIn=%s expectedOut=%s slippage=%.4f%%",
			exec.Chain(), tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), expectedOut, slippagePercent)
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
	if err := EnforceOkxSwapRouter("swap", cc.OKXSwapRouter, swapTx); err != nil {
		return nil, err
	}

	chainIDText := fmt.Sprintf("%d", cc.ChainID)
	approveSpender, err := s.okxService.GetApproveSpender(chainIDText, tokenIn.Hex())
	if err != nil {
		log.Printf("[Liquidity] Warning: failed to get OKX approve spender, using router as fallback: %v", err)
		approveSpender = to.Hex()
	}
	if !common.IsHexAddress(approveSpender) {
		return nil, fmt.Errorf("OKX approve spender invalid: %s", approveSpender)
	}
	approveAddr := common.HexToAddress(approveSpender)

	allowedSpenders := map[common.Address]struct{}{
		to:                        {},
		blockchain.Permit2Address: {},
	}
	if common.IsHexAddress(cc.OKXTokenApproveAddress) {
		allowedSpenders[common.HexToAddress(cc.OKXTokenApproveAddress)] = struct{}{}
	}
	if _, ok := allowedSpenders[approveAddr]; !ok {
		return nil, fmt.Errorf("OKX approve spender not allowed: %s (router=%s tokenApprove=%s)", approveAddr.Hex(), to.Hex(), strings.TrimSpace(cc.OKXTokenApproveAddress))
	}

	log.Printf("[Liquidity] OKX swap: chain=%s %s -> %s amount=%s router=%s approveTarget=%s",
		exec.Chain(), tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), to.Hex(), approveAddr.Hex())

	if approveAddr == blockchain.Permit2Address {
		if err := s.approveTokenViaPermit2(client, chainID, privateKey, walletAddr, tokenIn, to, amountIn, TxOptions{}); err != nil {
			return nil, fmt.Errorf("approve via Permit2 failed: %w", err)
		}
	} else {
		if err := s.approveToken(client, chainID, privateKey, walletAddr, tokenIn, approveAddr, amountIn, TxOptions{}); err != nil {
			return nil, fmt.Errorf("approve spender failed: %w", err)
		}
	}

	gasPrice, err := blockchain.GetGasPriceWithClient(client)
	if err != nil {
		return nil, err
	}

	gasLimit, err := okxSwapGasLimit(client, walletAddr, to, value, data, okxGasLimit)
	if err != nil {
		return nil, err
	}
	log.Printf("[Liquidity] OKX swap gasLimit: okx=%d final=%d", okxGasLimit, gasLimit)

	signed, err := blockchain.SendRawTransactionWithRetry(blockchain.SendRawTxParams{
		Client:     client,
		ChainID:    chainID,
		PrivateKey: privateKey,
		From:       walletAddr,
		To:         to,
		Value:      value,
		Data:       data,
		GasLimit:   gasLimit,
		GasPrice:   gasPrice,
	})
	if err != nil {
		return nil, err
	}

	txHash := signed.Hash().Hex()
	receipt, err := s.waitMined(client, chainID, signed)
	if err != nil {
		return nil, err
	}

	// Prefer receipt logs over balance reads to avoid public RPC stale reads.
	if d := ReceiptTokenTransferDelta(receipt, tokenOut, walletAddr); d != nil && d.Sign() > 0 {
		return &okxSwapExecutionResult{TxHash: txHash, Receipt: receipt, DeltaOut: d}, nil
	}

	outAfter, _ := blockchain.GetTokenBalanceWithClient(client, tokenOut, walletAddr)
	if outAfter == nil {
		outAfter = big.NewInt(0)
	}
	delta := new(big.Int).Sub(outAfter, outBefore)
	if delta.Sign() <= 0 {
		minWanted := new(big.Int).Add(outBefore, big.NewInt(1))
		if synced, werr := s.waitTokenBalanceAtLeast(client, tokenOut, walletAddr, minWanted, "OKX swap out"); werr == nil && synced != nil {
			outAfter = synced
			delta = new(big.Int).Sub(outAfter, outBefore)
		}
	}
	if delta.Sign() < 0 {
		delta = big.NewInt(0)
	}
	if delta.Sign() == 0 {
		log.Printf("[Liquidity] Warning: OKX swap mined but tokenOut delta is 0 (tx=%s tokenOut=%s expected=%s outBefore=%s outAfter=%s)",
			txHash, tokenOut.Hex(), expectedOut, outBefore.String(), outAfter.String())
	}

	return &okxSwapExecutionResult{TxHash: txHash, Receipt: receipt, DeltaOut: delta}, nil
}

// swapExactInViaOKX executes a swap transaction returned by OKX DEX /swap API from the user's wallet.
// It returns the balance delta of tokenOut observed on the wallet.
func (s *LiquidityService) swapExactInViaOKX(
	exec chainexec.EVMExecutor,
	privateKey *ecdsa.PrivateKey,
	walletAddr common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	slippagePercent float64,
) (*big.Int, error) {
	r, err := s.executeOKXSwapExactIn(exec, privateKey, walletAddr, tokenIn, tokenOut, amountIn, slippagePercent)
	if err != nil {
		return nil, err
	}
	if r == nil || r.DeltaOut == nil {
		return big.NewInt(0), nil
	}
	return r.DeltaOut, nil
}
