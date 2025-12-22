package services

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"gorm.io/gorm"
)

// LiquidityService handles liquidity operations
type LiquidityService struct {
	walletService      *WalletService
	okxService         *OKXDexService
	tradeRecordService *TradeRecordService
}

// NewLiquidityService creates a new liquidity service
func NewLiquidityService() *LiquidityService {
	return &LiquidityService{
		walletService:      NewWalletService(),
		okxService:         NewOKXDexService(),
		tradeRecordService: NewTradeRecordService(),
	}
}

// approveToken approves a token for spending
func (s *LiquidityService) approveToken(
	privateKey *ecdsa.PrivateKey,
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

	approveAmount := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

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

	tx, err := erc20.Approve(auth, spender, approveAmount)
	if err != nil {
		return fmt.Errorf("failed to approve: %w", err)
	}

	// Wait for approval tx to be mined to avoid racing subsequent calls.
	if _, err := s.waitMined(tx); err != nil {
		return fmt.Errorf("approve tx failed: %w", err)
	}

	return nil
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
