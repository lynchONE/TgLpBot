package wallet

import 
import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type BalanceSnapshotService struct {
	walletService *WalletService

	stopChan chan struct{}
	ticker   *time.Ticker
}

func NewBalanceSnapshotService() *BalanceSnapshotService {
	return &BalanceSnapshotService{
		walletService: NewWalletService(),
		stopChan:      make(chan struct{}),
		ticker:        time.NewTicker(30 * time.Minute),
	}
}

func (s *BalanceSnapshotService) Start() {
	go s.run()
}

func (s *BalanceSnapshotService) Stop() {
	close(s.stopChan)
	s.ticker.Stop()
}

func (s *BalanceSnapshotService) CaptureTodayForUser(userID uint) error {
	wallet, err := s.walletService.GetDefaultWallet(userID)
	if err != nil {
		return err
	}
	return s.CaptureForWallet(userID, wallet.Address, time.Now().Format("2006-01-02"))
}

func (s *BalanceSnapshotService) CaptureForWallet(userID uint, walletAddress string, day string) error {
	if config.AppConfig == nil {
		return fmt.Errorf("config not loaded")
	}
	if blockchain.Client == nil {
		return fmt.Errorf("blockchain client not initialized")
	}

	walletAddress = strings.TrimSpace(walletAddress)
	if !common.IsHexAddress(walletAddress) {
		return fmt.Errorf("invalid wallet address: %s", walletAddress)
	}
	addr := common.HexToAddress(walletAddress)

	usdtAddr := common.HexToAddress(config.AppConfig.USDTAddress)
	usdtBal, _ := blockchain.GetTokenBalance(usdtAddr, addr)
	if usdtBal == nil {
		usdtBal = big.NewInt(0)
	}
	bnbBal, _ := blockchain.GetBalance(addr)
	if bnbBal == nil {
		bnbBal = big.NewInt(0)
	}

	base := models.WalletBalanceSnapshot{
		UserID:        userID,
		WalletAddress: walletAddress,
		Day:           strings.TrimSpace(day),
	}
	assign := models.WalletBalanceSnapshot{
		BNBBalanceWei:  bnbBal.String(),
		USDTBalanceWei: usdtBal.String(),
	}

	return database.DB.Where(&base).Assign(assign).FirstOrCreate(&base).Error
}

func (s *BalanceSnapshotService) run() {
	lastDay := time.Now().Format("2006-01-02")
	if err := s.CaptureDayForAllUsers(lastDay); err != nil {
		log.Printf("[BalanceSnapshot] capture day failed: %v", err)
	}
	for {
		select {
		case <-s.stopChan:
			return
		case <-s.ticker.C:
			day := time.Now().Format("2006-01-02")
			if day == lastDay {
				continue
			}
			if err := s.CaptureDayForAllUsers(day); err != nil {
				log.Printf("[BalanceSnapshot] capture day failed: %v", err)
			} else {
				lastDay = day
			}
		}
	}
}

func (s *BalanceSnapshotService) CaptureDayForAllUsers(day string) error {
	if database.DB == nil {
		return fmt.Errorf("db not initialized")
	}

	var wallets []models.Wallet
	if err := database.DB.Order("user_id ASC, is_default DESC, id ASC").Find(&wallets).Error; err != nil {
		return err
	}

	// Pick one wallet per user: default first, otherwise the first wallet.
	picked := make(map[uint]models.Wallet)
	for _, w := range wallets {
		if w.UserID == 0 {
			continue
		}
		if _, ok := picked[w.UserID]; ok {
			continue
		}
		picked[w.UserID] = w
	}

	for userID, w := range picked {
		if err := s.CaptureForWallet(userID, w.Address, day); err != nil {
			log.Printf("[BalanceSnapshot] capture user=%d wallet=%s failed: %v", userID, w.Address, err)
		}
	}
	return nil
}
