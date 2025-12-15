package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"gorm.io/gorm"
)

// LiquidityService handles liquidity operations
type LiquidityService struct {
	walletService *WalletService
	okxService    *OKXDexService
}

// NewLiquidityService creates a new liquidity service
func NewLiquidityService() *LiquidityService {
	return &LiquidityService{
		walletService: NewWalletService(),
		okxService:    NewOKXDexService(),
	}
}

// AddLiquidityWithUSDT adds liquidity using USDT
func (s *LiquidityService) AddLiquidityWithUSDT(
	userID uint,
	poolAddress string,
	usdtAmount *big.Int,
	slippage float64,
) (string, error) {
	// Get user's default wallet
	wallet, err := s.walletService.GetDefaultWallet(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get wallet: %w", err)
	}
	
	// Get private key
	privateKeyHex, err := s.walletService.GetPrivateKey(wallet)
	if err != nil {
		return "", fmt.Errorf("failed to get private key: %w", err)
	}
	
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}
	
	walletAddr := s.walletService.GetWalletAddress(wallet)
	
	// Get pool info
	lpConfig, err := s.GetLPConfig(userID, poolAddress)
	if err != nil {
		return "", fmt.Errorf("failed to get LP config: %w", err)
	}
	
	// Determine which token to swap USDT for
	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)
	token0Addr := common.HexToAddress(lpConfig.Token0Address)
	token1Addr := common.HexToAddress(lpConfig.Token1Address)
	
	var targetTokenAddr common.Address
	if token0Addr == usdtAddr {
		targetTokenAddr = token1Addr
	} else if token1Addr == usdtAddr {
		targetTokenAddr = token0Addr
	} else {
		// Neither token is USDT, need to swap USDT to one of them
		targetTokenAddr = token0Addr
	}
	
	// Step 1: Approve USDT to Zap contract
	zapAddr := common.HexToAddress(config.AppConfig.ZapContractAddress)
	if err := s.approveToken(privateKey, walletAddr, usdtAddr, zapAddr, usdtAmount); err != nil {
		return "", fmt.Errorf("failed to approve USDT: %w", err)
	}
	
	// Step 2: Use Zap contract to add liquidity
	txHash, err := s.zapIn(privateKey, walletAddr, usdtAddr, usdtAmount, common.HexToAddress(poolAddress), slippage)
	if err != nil {
		return "", fmt.Errorf("failed to zap in: %w", err)
	}
	
	// Record transaction
	tx := &models.Transaction{
		UserID:          userID,
		TxHash:          txHash,
		Type:            models.TxTypeAddLiquidity,
		Status:          models.TxStatusPending,
		FromAddress:     walletAddr.Hex(),
		ToAddress:       zapAddr.Hex(),
		TokenInAddress:  usdtAddr.Hex(),
		TokenOutAddress: poolAddress,
		AmountIn:        usdtAmount.String(),
	}
	
	if err := database.DB.Create(tx).Error; err != nil {
		return txHash, fmt.Errorf("failed to record transaction: %w", err)
	}
	
	return txHash, nil
}

// RemoveLiquidityToUSDT removes liquidity and converts to USDT
func (s *LiquidityService) RemoveLiquidityToUSDT(
	userID uint,
	poolAddress string,
	lpAmount *big.Int,
	slippage float64,
) (string, error) {
	// Get user's default wallet
	wallet, err := s.walletService.GetDefaultWallet(userID)
	if err != nil {
		return "", fmt.Errorf("failed to get wallet: %w", err)
	}
	
	// Get private key
	privateKeyHex, err := s.walletService.GetPrivateKey(wallet)
	if err != nil {
		return "", fmt.Errorf("failed to get private key: %w", err)
	}
	
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}
	
	walletAddr := s.walletService.GetWalletAddress(wallet)
	zapAddr := common.HexToAddress(config.AppConfig.ZapContractAddress)
	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)
	poolAddr := common.HexToAddress(poolAddress)
	
	// Step 1: Approve LP token to Zap contract
	if err := s.approveToken(privateKey, walletAddr, poolAddr, zapAddr, lpAmount); err != nil {
		return "", fmt.Errorf("failed to approve LP token: %w", err)
	}
	
	// Step 2: Use Zap contract to remove liquidity
	txHash, err := s.zapOut(privateKey, walletAddr, poolAddr, lpAmount, usdtAddr, slippage)
	if err != nil {
		return "", fmt.Errorf("failed to zap out: %w", err)
	}
	
	// Record transaction
	tx := &models.Transaction{
		UserID:          userID,
		TxHash:          txHash,
		Type:            models.TxTypeRemoveLiquidity,
		Status:          models.TxStatusPending,
		FromAddress:     walletAddr.Hex(),
		ToAddress:       zapAddr.Hex(),
		TokenInAddress:  poolAddress,
		TokenOutAddress: usdtAddr.Hex(),
		AmountIn:        lpAmount.String(),
	}
	
	if err := database.DB.Create(tx).Error; err != nil {
		return txHash, fmt.Errorf("failed to record transaction: %w", err)
	}
	
	return txHash, nil
}

// approveToken approves a token for spending
func (s *LiquidityService) approveToken(
	privateKey *crypto.PrivateKey,
	from, token, spender common.Address,
	amount *big.Int,
) error {
	// Check current allowance
	erc20, err := blockchain.NewERC20(token, blockchain.Client)
	if err != nil {
		return err
	}
	
	allowance, err := erc20.Allowance(nil, from, spender)
	if err != nil {
		return fmt.Errorf("failed to get allowance: %w", err)
	}
	
	// If allowance is sufficient, no need to approve
	if allowance.Cmp(amount) >= 0 {
		return nil
	}
	
	// Create approve transaction
	nonce, err := blockchain.GetNonce(from)
	if err != nil {
		return err
	}
	
	gasPrice, err := blockchain.GetGasPrice()
	if err != nil {
		return err
	}
	
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, blockchain.ChainID)
	if err != nil {
		return err
	}
	
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)
	auth.GasLimit = config.AppConfig.GasLimit
	auth.GasPrice = gasPrice
	
	_, err = erc20.Approve(auth, spender, amount)
	if err != nil {
		return fmt.Errorf("failed to approve: %w", err)
	}
	
	// Wait a bit for approval to be mined
	time.Sleep(3 * time.Second)
	
	return nil
}

// zapIn adds liquidity using Zap contract
func (s *LiquidityService) zapIn(
	privateKey *crypto.PrivateKey,
	from, tokenIn common.Address,
	amountIn *big.Int,
	pair common.Address,
	slippage float64,
) (string, error) {
	zapAddr := common.HexToAddress(config.AppConfig.ZapContractAddress)
	zap, err := blockchain.NewLiquidityZap(zapAddr, blockchain.Client)
	if err != nil {
		return "", err
	}
	
	nonce, err := blockchain.GetNonce(from)
	if err != nil {
		return "", err
	}
	
	gasPrice, err := blockchain.GetGasPrice()
	if err != nil {
		return "", err
	}
	
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, blockchain.ChainID)
	if err != nil {
		return "", err
	}
	
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)
	auth.GasLimit = config.AppConfig.GasLimit
	auth.GasPrice = gasPrice
	
	// Calculate minimum liquidity (with slippage)
	minLiquidity := big.NewInt(0) // Can be calculated based on expected output
	
	// Deadline: 20 minutes from now
	deadline := big.NewInt(time.Now().Add(20 * time.Minute).Unix())
	
	_, err = zap.ZapIn(auth, tokenIn, amountIn, pair, minLiquidity, deadline)
	if err != nil {
		return "", fmt.Errorf("failed to execute zapIn: %w", err)
	}
	
	// Get transaction hash from auth
	// Note: This is a simplified version, actual implementation may vary
	return "", nil
}

// zapOut removes liquidity using Zap contract
func (s *LiquidityService) zapOut(
	privateKey *crypto.PrivateKey,
	from, pair common.Address,
	liquidity *big.Int,
	tokenOut common.Address,
	slippage float64,
) (string, error) {
	zapAddr := common.HexToAddress(config.AppConfig.ZapContractAddress)
	zap, err := blockchain.NewLiquidityZap(zapAddr, blockchain.Client)
	if err != nil {
		return "", err
	}
	
	nonce, err := blockchain.GetNonce(from)
	if err != nil {
		return "", err
	}
	
	gasPrice, err := blockchain.GetGasPrice()
	if err != nil {
		return "", err
	}
	
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, blockchain.ChainID)
	if err != nil {
		return "", err
	}
	
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)
	auth.GasLimit = config.AppConfig.GasLimit
	auth.GasPrice = gasPrice
	
	// Calculate minimum amount out (with slippage)
	minAmountOut := big.NewInt(0) // Can be calculated based on expected output
	
	// Deadline: 20 minutes from now
	deadline := big.NewInt(time.Now().Add(20 * time.Minute).Unix())
	
	_, err = zap.ZapOut(auth, pair, liquidity, tokenOut, minAmountOut, deadline)
	if err != nil {
		return "", fmt.Errorf("failed to execute zapOut: %w", err)
	}
	
	return "", nil
}

// GetLPConfig gets LP configuration for a pool
func (s *LiquidityService) GetLPConfig(userID uint, poolAddress string) (*models.LPConfig, error) {
	var config models.LPConfig
	err := database.DB.Where("user_id = ? AND pool_address = ?", userID, poolAddress).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("LP config not found")
		}
		return nil, err
	}
	return &config, nil
}

// CreateLPConfig creates a new LP configuration
func (s *LiquidityService) CreateLPConfig(config *models.LPConfig) error {
	return database.DB.Create(config).Error
}

// UpdateLPConfig updates an LP configuration
func (s *LiquidityService) UpdateLPConfig(config *models.LPConfig) error {
	return database.DB.Save(config).Error
}

