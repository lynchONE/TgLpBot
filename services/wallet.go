package services

import (
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"gorm.io/gorm"
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
	return s.decryptPrivateKey(wallet.EncryptedPrivateKey)
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
	key, err := hex.DecodeString(config.AppConfig.EncryptionKey)
	if err != nil {
		return "", fmt.Errorf("invalid encryption key: %w", err)
	}
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	ciphertext := gcm.Seal(nonce, nonce, []byte(privateKeyHex), nil)
	return hex.EncodeToString(ciphertext), nil
}

// decryptPrivateKey decrypts a private key using AES
func (s *WalletService) decryptPrivateKey(encryptedHex string) (string, error) {
	key, err := hex.DecodeString(config.AppConfig.EncryptionKey)
	if err != nil {
		return "", fmt.Errorf("invalid encryption key: %w", err)
	}
	
	ciphertext, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted data: %w", err)
	}
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	
	return string(plaintext), nil
}

// GetWalletAddress returns the address as common.Address
func (s *WalletService) GetWalletAddress(wallet *models.Wallet) common.Address {
	return common.HexToAddress(wallet.Address)
}

