package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

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
		s.okxService = NewOKXDexService()
	}

	outBefore, _ := blockchain.GetTokenBalance(tokenOut, walletAddr)
	if outBefore == nil {
		outBefore = big.NewInt(0)
	}

	swapResp, err := s.okxService.GetSwapData(SwapRequest{
		ChainID:           fmt.Sprintf("%d", config.AppConfig.BSCChainID),
		FromTokenAddress:  tokenIn.Hex(),
		ToTokenAddress:    tokenOut.Hex(),
		Amount:            amountIn.String(),
		Slippage:          s.okxSlippageDecimal(slippagePercent),
		UserWalletAddress: walletAddr.Hex(),
	})
	if err != nil {
		return nil, err
	}
	if len(swapResp.Data) == 0 {
		return nil, fmt.Errorf("OKX swap response empty")
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

	gasLimit := config.AppConfig.GasLimit
	if strings.TrimSpace(txObj.Gas) != "" {
		if g, ok := new(big.Int).SetString(strings.TrimSpace(txObj.Gas), 10); ok && g.IsUint64() {
			gasLimit = g.Uint64()
		}
	}

	swapTx := blockchain.OkxSwapTx{To: to, Value: value, Data: data}
	_ = validateOkxSmartSwapTx("swap", swapTx)
	if err := enforceOkxSwapRouter("swap", swapTx); err != nil {
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

	rawTx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, data)
	signed, err := types.SignTx(rawTx, types.NewEIP155Signer(blockchain.ChainID), privateKey)
	if err != nil {
		return nil, err
	}
	if _, err := blockchain.SendTransaction(signed); err != nil {
		return nil, err
	}
	if _, err := s.waitMined(signed); err != nil {
		return nil, err
	}

	outAfter, _ := blockchain.GetTokenBalance(tokenOut, walletAddr)
	if outAfter == nil {
		outAfter = big.NewInt(0)
	}
	delta := new(big.Int).Sub(outAfter, outBefore)
	if delta.Sign() < 0 {
		delta = big.NewInt(0)
	}
	return delta, nil
}
