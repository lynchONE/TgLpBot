package smart_money_watch_open_alert

import (
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/base/notify"
	smgd "TgLpBot/service/smart_money_golden_dog"
	"context"
	"fmt"
	"log"
	"strings"
)

type Service struct {
	repo *Repository
}

func NewService() *Service {
	return &Service{repo: NewRepository()}
}

func (s *Service) Repo() *Repository {
	if s == nil {
		return nil
	}
	return s.repo
}

func (s *Service) HandleEvent(ctx context.Context, event *models.SmartMoneyLPEvent, walletLabel *string) {
	if s == nil || s.repo == nil || event == nil {
		return
	}
	if strings.ToLower(strings.TrimSpace(event.EventType)) != "add" {
		return
	}

	chain := normalizeChain(chainFromID(event.ChainID))
	walletAddress := normalizeWalletAddress(event.WalletAddress)
	if walletAddress == "" {
		return
	}

	userIDs, err := s.repo.ListWatcherUserIDs(ctx, chain, walletAddress)
	if err != nil {
		log.Printf("[SmartMoney WatchOpenAlert] list watchers failed wallet=%s chain=%s err=%v", walletAddress, chain, err)
		return
	}
	if len(userIDs) == 0 {
		return
	}

	barkCache := make(map[uint]smgd.BarkStatus)
	for _, userID := range userIDs {
		cfg, err := s.repo.GetOrCreateConfig(ctx, userID, chain)
		if err != nil {
			log.Printf("[SmartMoney WatchOpenAlert] load config failed user=%d chain=%s err=%v", userID, chain, err)
			continue
		}
		if !cfg.Enabled || !cfg.BarkEnabled {
			continue
		}

		claimed, err := s.repo.ClaimReceipt(ctx, userID, chain, event.TxHash, event.LogIndex)
		if err != nil {
			log.Printf("[SmartMoney WatchOpenAlert] claim receipt failed user=%d tx=%s log=%d err=%v", userID, event.TxHash, event.LogIndex, err)
			continue
		}
		if !claimed {
			continue
		}

		barkStatus, err := resolveReadyBarkStatus(ctx, barkCache, userID)
		if err != nil {
			log.Printf("[SmartMoney WatchOpenAlert] resolve bark failed user=%d err=%v", userID, err)
			continue
		}
		if !barkStatus.Ready {
			continue
		}

		title, body := buildWatchedOpenBarkMessage(event, walletLabel)
		barkCfg := barkStatus.Config
		barkCfg.OpenURL = explorerTxURL(chain, event.TxHash)
		if err := notify.SendBarkWithConfig(title, body, barkCfg); err != nil {
			_ = s.repo.DeleteReceipt(ctx, userID, chain, event.TxHash, event.LogIndex)
			log.Printf("[SmartMoney WatchOpenAlert] bark notify failed user=%d tx=%s err=%v", userID, event.TxHash, err)
		}
	}
}

func resolveReadyBarkStatus(ctx context.Context, cache map[uint]smgd.BarkStatus, userID uint) (smgd.BarkStatus, error) {
	if status, ok := cache[userID]; ok {
		return status, nil
	}
	status, err := smgd.ResolveUserBarkStatus(ctx, userID)
	if err != nil {
		return smgd.BarkStatus{}, err
	}
	cache[userID] = status
	return status, nil
}

func chainFromID(chainID int) string {
	switch chainID {
	case 8453:
		return "base"
	default:
		return "bsc"
	}
}

func explorerTxURL(chain string, txHash string) string {
	if url := config.ExplorerTxURL(chain, txHash); strings.TrimSpace(url) != "" {
		return url
	}
	switch normalizeChain(chain) {
	case "base":
		return fmt.Sprintf("https://basescan.org/tx/%s", txHash)
	default:
		return fmt.Sprintf("https://bscscan.com/tx/%s", txHash)
	}
}

func buildWatchedOpenBarkMessage(event *models.SmartMoneyLPEvent, walletLabel *string) (string, string) {
	walletName := firstNonEmpty(labelValue(walletLabel), shortenAddress(event.WalletAddress))
	pairName := firstNonEmpty(
		buildPairLabel(event.Token0Symbol, event.Token1Symbol),
		shortenAddress(event.PoolAddress),
	)
	chain := strings.ToUpper(normalizeChain(chainFromID(event.ChainID)))
	txSuffix := shortenAddress(event.TxHash)
	title := "Watched Smart Money Open"
	body := fmt.Sprintf("%s opened %s | chain %s | tx %s", walletName, pairName, chain, txSuffix)
	return title, body
}

func buildPairLabel(token0Symbol string, token1Symbol string) string {
	left := strings.TrimSpace(token0Symbol)
	right := strings.TrimSpace(token1Symbol)
	switch {
	case left != "" && right != "":
		return left + "/" + right
	case left != "":
		return left
	case right != "":
		return right
	default:
		return ""
	}
}

func labelValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func shortenAddress(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 10 {
		return value
	}
	return value[:6] + "..." + value[len(value)-4:]
}
