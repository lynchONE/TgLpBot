package liquidity

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/exchange"
	"TgLpBot/service/trade"
	"TgLpBot/service/wallet"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"gorm.io/gorm"
)

// LiquidityService handles liquidity operations
type LiquidityService struct {
	walletService      *wallet.WalletService
	okxService         *exchange.OKXDexService
	tradeRecordService *trade.TradeRecordService
}

type TxOptions struct {
	// GasMultiplier multiplies suggested gasPrice (e.g. 2.0 for emergency). 0 means 1.0.
	GasMultiplier float64
	// ExitPercent optionally limits a liquidity exit to a percentage of current position liquidity.
	// nil means the existing full-exit behavior (100%).
	ExitPercent *float64
	// EntrySwapSlippageOverride only applies to the initial wallet-side entry swap before opening.
	// It must not change the task's persisted slippage tolerance.
	EntrySwapSlippageOverride *float64
	// SlippageToleranceOverride applies to one liquidity operation without changing the task record.
	SlippageToleranceOverride *float64
}

func normalizeGasMultiplier(v float64) float64 {
	if v <= 0 {
		return 1
	}
	if v > 10 {
		return 10
	}
	return v
}

// NewLiquidityService creates a new liquidity service
func NewLiquidityService() *LiquidityService {
	return &LiquidityService{
		walletService:      wallet.NewWalletService(),
		okxService:         exchange.NewOKXDexService(),
		tradeRecordService: trade.NewTradeRecordService(),
	}
}

// approveToken approves a token for spending
func (s *LiquidityService) approveToken(
	client *ethclient.Client,
	chainID *big.Int,
	privateKey *ecdsa.PrivateKey,
	from, token, spender common.Address,
	amount *big.Int,
	opts TxOptions,
) error {
	if client == nil || chainID == nil {
		return fmt.Errorf("blockchain client not initialized")
	}
	// Check current allowance
	erc20, err := blockchain.NewERC20(token, client)
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

	// Some tokens (e.g. USDT-style) require allowance to be set to 0 before changing from a non-zero value.
	// Use a "forceApprove" pattern when current allowance is non-zero.
	if allowance.Sign() > 0 {
		nonce0, err := blockchain.GetNonceWithClient(client, from)
		if err != nil {
			return err
		}
		auth0, err := s.buildAuth(client, chainID, privateKey, nonce0, big.NewInt(0), opts)
		if err != nil {
			return err
		}
		tx0, err := erc20.Approve(auth0, spender, big.NewInt(0))
		if err != nil {
			return fmt.Errorf("failed to reset allowance to 0: %w", err)
		}
		if _, err := s.waitMined(client, chainID, tx0); err != nil {
			return fmt.Errorf("reset allowance tx failed: %w", err)
		}
	}

	nonce, err := blockchain.GetNonceWithClient(client, from)
	if err != nil {
		return err
	}
	auth, err := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
	if err != nil {
		return err
	}

	tx, err := erc20.Approve(auth, spender, approveAmount)
	if err != nil {
		return fmt.Errorf("failed to approve: %w", err)
	}

	// Wait for approval tx to be mined to avoid racing subsequent calls.
	if _, err := s.waitMined(client, chainID, tx); err != nil {
		return fmt.Errorf("approve tx failed: %w", err)
	}

	return nil
}

// approveTokenViaPermit2 approves `spender` to spend `token` from `from` via Permit2 (finite allowance).
// This is needed for routers that pull tokens through Permit2 rather than ERC20 allowance.
func (s *LiquidityService) approveTokenViaPermit2(
	client *ethclient.Client,
	chainID *big.Int,
	privateKey *ecdsa.PrivateKey,
	from common.Address,
	token common.Address,
	spender common.Address,
	amount *big.Int,
	opts TxOptions,
) error {
	if client == nil || chainID == nil {
		return fmt.Errorf("blockchain client not initialized")
	}
	if amount == nil || amount.Sign() <= 0 {
		return nil
	}

	// 1) Ensure ERC20 allowance: token -> Permit2
	if err := s.approveToken(client, chainID, privateKey, from, token, blockchain.Permit2Address, amount, opts); err != nil {
		return fmt.Errorf("approve token->Permit2 failed: %w", err)
	}

	// 2) Ensure Permit2 allowance: owner -> spender (finite)
	permit2, err := blockchain.NewPermit2(blockchain.Permit2Address, client)
	if err != nil {
		return fmt.Errorf("init Permit2 failed: %w", err)
	}
	allow, err := permit2.Allowance(nil, from, token, spender)
	if err != nil {
		return fmt.Errorf("failed to get Permit2 allowance: %w", err)
	}

	maxAmount := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 160), big.NewInt(1))
	maxExpiration := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 48), big.NewInt(1))

	now := time.Now().Unix()
	if allow != nil && allow.Amount != nil && allow.Expiration != nil {
		// If already infinite, do nothing (some Permit2 variants revert if you try to change infinity).
		if allow.Amount.Cmp(maxAmount) == 0 && allow.Expiration.Cmp(maxExpiration) == 0 {
			return nil
		}
		// Otherwise, if the existing finite allowance is sufficient and unexpired, keep it.
		if allow.Amount.Cmp(amount) >= 0 && allow.Expiration.Sign() > 0 && allow.Expiration.Int64() > now {
			return nil
		}
	}

	tryApprove := func(apprAmount *big.Int, apprExpiration *big.Int) error {
		nonce, nerr := blockchain.GetNonceWithClient(client, from)
		if nerr != nil {
			return nerr
		}
		auth, aerr := s.buildAuth(client, chainID, privateKey, nonce, big.NewInt(0), opts)
		if aerr != nil {
			return aerr
		}
		tx, terr := permit2.Approve(auth, token, spender, apprAmount, apprExpiration)
		if terr != nil {
			// Permit2AllowanceIsFixedAtInfinity()
			if strings.Contains(terr.Error(), permit2AllowanceIsFixedAtInfinitySelector) {
				// Treat as success only if the on-chain allowance is already infinite.
				if a2, aerr := permit2.Allowance(nil, from, token, spender); aerr == nil && a2 != nil &&
					a2.Amount != nil && a2.Expiration != nil &&
					a2.Amount.Cmp(maxAmount) == 0 && a2.Expiration.Cmp(maxExpiration) == 0 {
					return nil
				}
			}
			return terr
		}
		if _, werr := s.waitMined(client, chainID, tx); werr != nil {
			if strings.Contains(werr.Error(), permit2AllowanceIsFixedAtInfinitySelector) {
				if a2, aerr := permit2.Allowance(nil, from, token, spender); aerr == nil && a2 != nil &&
					a2.Amount != nil && a2.Expiration != nil &&
					a2.Amount.Cmp(maxAmount) == 0 && a2.Expiration.Cmp(maxExpiration) == 0 {
					return nil
				}
			}
			return werr
		}
		return nil
	}

	// 1) Prefer finite allowance (what wallets show as "finite Permit2 approve").
	finiteExpiration := big.NewInt(now + int64(30*24*60*60)) // 30 days
	if err := tryApprove(amount, finiteExpiration); err == nil {
		return nil
	}

	// 2) Fallback: some tokens/routers require Permit2 allowance to be fixed at infinity.
	if err2 := tryApprove(maxAmount, maxExpiration); err2 != nil {
		return fmt.Errorf("permit2 approve failed (finite+infinite attempts): %w", err2)
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
