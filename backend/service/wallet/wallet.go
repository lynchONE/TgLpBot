package wallet

import (
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gorm.io/gorm"

	"TgLpBot/base/security"
)

// WalletService handles wallet operations
type WalletService struct{}

// NewWalletService creates a new wallet service
func NewWalletService() *WalletService {
	return &WalletService{}
}

// CreateWallet creates a new wallet for a user
func (s *WalletService) CreateWallet(userID uint, name string) (*models.Wallet, error) {
	// Generate new private key
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Get address from private key
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Encrypt private key
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))
	encryptedPrivateKey, err := s.encryptPrivateKey(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key: %w", err)
	}

	// Create wallet record
	wallet := &models.Wallet{
		UserID:              userID,
		Address:             address.Hex(),
		EncryptedPrivateKey: encryptedPrivateKey,
		Name:                name,
		IsDefault:           false,
	}

	if err := database.DB.Create(wallet).Error; err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	return wallet, nil
}

// ImportWallet imports an existing wallet
func (s *WalletService) ImportWallet(userID uint, privateKeyHex, name string) (*models.Wallet, error) {
	// Validate private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	// Get address from private key
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Check if wallet already exists
	var existingWallet models.Wallet
	err = database.DB.Where("address = ?", address.Hex()).First(&existingWallet).Error
	if err == nil {
		return nil, errors.New("wallet already exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing wallet: %w", err)
	}

	// Encrypt private key
	encryptedPrivateKey, err := s.encryptPrivateKey(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key: %w", err)
	}

	// Create wallet record
	wallet := &models.Wallet{
		UserID:              userID,
		Address:             address.Hex(),
		EncryptedPrivateKey: encryptedPrivateKey,
		Name:                name,
		IsDefault:           false,
	}

	if err := database.DB.Create(wallet).Error; err != nil {
		return nil, fmt.Errorf("failed to import wallet: %w", err)
	}

	return wallet, nil
}

// GetUserWallets returns all wallets for a user
func (s *WalletService) GetUserWallets(userID uint) ([]models.Wallet, error) {
	var wallets []models.Wallet
	err := database.DB.Where("user_id = ?", userID).Find(&wallets).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get wallets: %w", err)
	}
	return wallets, nil
}

// GetWalletByID returns a specific wallet for a user.
func (s *WalletService) GetWalletByID(userID uint, walletID uint) (*models.Wallet, error) {
	if userID == 0 || walletID == 0 {
		return nil, errors.New("invalid user_id or wallet_id")
	}
	var w models.Wallet
	if err := database.DB.Where("id = ? AND user_id = ?", walletID, userID).First(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("wallet not found")
		}
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	return &w, nil
}

func normalizeWalletAddressForQuery(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	if !common.IsHexAddress(v) {
		return "", false
	}
	return common.HexToAddress(v).Hex(), true
}

// GetWalletByAddress returns a wallet for a user by its address.
func (s *WalletService) GetWalletByAddress(userID uint, address string) (*models.Wallet, error) {
	if userID == 0 {
		return nil, errors.New("invalid user_id")
	}
	addr, ok := normalizeWalletAddressForQuery(address)
	if !ok {
		return nil, errors.New("invalid wallet address")
	}

	var w models.Wallet
	if err := database.DB.Where("user_id = ? AND address = ?", userID, addr).First(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("wallet not found")
		}
		return nil, fmt.Errorf("failed to get wallet: %w", err)
	}
	return &w, nil
}

// ResolveTaskWallet returns the wallet bound to a task (wallet_id or wallet_address).
// If neither is set, it falls back to the user's default wallet (legacy tasks).
func (s *WalletService) ResolveTaskWallet(userID uint, walletID uint, walletAddress string) (*models.Wallet, error) {
	if userID == 0 {
		return nil, errors.New("invalid user_id")
	}
	if walletID != 0 {
		return s.GetWalletByID(userID, walletID)
	}
	if strings.TrimSpace(walletAddress) != "" {
		return s.GetWalletByAddress(userID, walletAddress)
	}
	return s.GetDefaultWallet(userID)
}

// GetDefaultWallet returns the default wallet for a user
func (s *WalletService) GetDefaultWallet(userID uint) (*models.Wallet, error) {
	var wallet models.Wallet
	err := database.DB.Where("user_id = ? AND is_default = ?", userID, true).First(&wallet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// If no default wallet, get the first wallet
			err = database.DB.Where("user_id = ?", userID).First(&wallet).Error
			if err != nil {
				return nil, fmt.Errorf("no wallet found: %w", err)
			}
			return &wallet, nil
		}
		return nil, fmt.Errorf("failed to get default wallet: %w", err)
	}
	return &wallet, nil
}

// SetDefaultWallet sets a wallet as default
func (s *WalletService) SetDefaultWallet(userID uint, walletID uint) error {
	// Unset all default wallets for user
	if err := database.DB.Model(&models.Wallet{}).Where("user_id = ?", userID).Update("is_default", false).Error; err != nil {
		return fmt.Errorf("failed to unset default wallets: %w", err)
	}

	// Set new default wallet
	if err := database.DB.Model(&models.Wallet{}).Where("id = ? AND user_id = ?", walletID, userID).Update("is_default", true).Error; err != nil {
		return fmt.Errorf("failed to set default wallet: %w", err)
	}

	return nil
}

// GetPrivateKey decrypts and returns the private key
func (s *WalletService) GetPrivateKey(wallet *models.Wallet) (string, error) {
	if wallet == nil {
		return "", errors.New("wallet is nil")
	}

	plain, err := s.decryptPrivateKey(wallet.EncryptedPrivateKey)
	if err == nil {
		return plain, nil
	}

	// Backward-compat / migration: if the DB contains plaintext private keys, re-encrypt in-place.
	candidate := normalizeHexPrivateKey(wallet.EncryptedPrivateKey)
	if _, kerr := crypto.HexToECDSA(candidate); kerr != nil {
		return "", err
	}
	encrypted, eerr := s.encryptPrivateKey(candidate)
	if eerr != nil {
		return "", fmt.Errorf("failed to encrypt legacy plaintext key: %w", eerr)
	}
	if database.DB != nil && wallet.ID != 0 {
		_ = database.DB.Model(&models.Wallet{}).Where("id = ?", wallet.ID).Update("encrypted_private_key", encrypted).Error
	}
	wallet.EncryptedPrivateKey = encrypted
	return candidate, nil
}

// MigratePlaintextPrivateKeys encrypts any legacy plaintext keys found in the wallets table.
// It is safe to run multiple times.
func (s *WalletService) MigratePlaintextPrivateKeys() (int, error) {
	if database.DB == nil {
		return 0, errors.New("database not initialized")
	}

	var wallets []models.Wallet
	if err := database.DB.Find(&wallets).Error; err != nil {
		return 0, fmt.Errorf("load wallets failed: %w", err)
	}

	migrated := 0
	for i := range wallets {
		w := &wallets[i]
		if w == nil {
			continue
		}

		plain, err := s.decryptPrivateKey(w.EncryptedPrivateKey)
		if err == nil {
			plain = normalizeHexPrivateKey(plain)
			if _, kerr := crypto.HexToECDSA(plain); kerr != nil {
				return migrated, fmt.Errorf("wallet %d has invalid decrypted private key", w.ID)
			}
			continue
		}

		candidate := normalizeHexPrivateKey(w.EncryptedPrivateKey)
		if _, kerr := crypto.HexToECDSA(candidate); kerr != nil {
			// Not decryptable and not a valid plaintext key => refuse to guess.
			return migrated, fmt.Errorf("wallet %d (%s) private key is unreadable (decrypt failed)", w.ID, w.Address)
		}

		encrypted, eerr := s.encryptPrivateKey(candidate)
		if eerr != nil {
			return migrated, fmt.Errorf("wallet %d encrypt failed: %w", w.ID, eerr)
		}

		if uerr := database.DB.Model(&models.Wallet{}).Where("id = ?", w.ID).Update("encrypted_private_key", encrypted).Error; uerr != nil {
			return migrated, fmt.Errorf("wallet %d update failed: %w", w.ID, uerr)
		}
		migrated++
	}

	return migrated, nil
}

// RenameWallet renames a wallet
func (s *WalletService) RenameWallet(userID uint, walletID uint, name string) error {
	result := database.DB.Model(&models.Wallet{}).Where("id = ? AND user_id = ?", walletID, userID).Update("name", name)
	if result.Error != nil {
		return fmt.Errorf("failed to rename wallet: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("wallet not found")
	}
	return nil
}

// DeleteWallet deletes a wallet
func (s *WalletService) DeleteWallet(userID uint, walletID uint) error {
	result := database.DB.Where("id = ? AND user_id = ?", walletID, userID).Delete(&models.Wallet{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete wallet: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("wallet not found")
	}
	return nil
}

// encryptPrivateKey encrypts a private key using AES
func (s *WalletService) encryptPrivateKey(privateKeyHex string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}
	privateKeyHex = normalizeHexPrivateKey(privateKeyHex)
	return security.EncryptAESGCMToHex(key, []byte(privateKeyHex))
}

// decryptPrivateKey decrypts a private key using AES
func (s *WalletService) decryptPrivateKey(encryptedHex string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}
	plaintext, err := security.DecryptAESGCMHex(key, encryptedHex)
	if err != nil {
		return "", err
	}
	return normalizeHexPrivateKey(string(plaintext)), nil
}

// GetWalletAddress returns the address as common.Address
func (s *WalletService) GetWalletAddress(wallet *models.Wallet) common.Address {
	return common.HexToAddress(wallet.Address)
}

func getEncryptionKey() ([]byte, error) {
	if config.AppConfig == nil {
		return nil, errors.New("config not loaded")
	}
	key, err := security.DecodeHexKey32(config.AppConfig.EncryptionKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func normalizeHexPrivateKey(s string) string {
	s = strings.TrimSpace(s)
	s = security.NormalizeHexString(s)
	return s
}
