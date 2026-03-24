package smart_money

import (
	"TgLpBot/base/models"
	"fmt"
	"strconv"
	"strings"
)

func NormalizePositionRef(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func BuildPositionRef(chainID int, protocol, wallet string, nftTokenID uint64, pool string, tickLower, tickUpper *int) string {
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	wallet = strings.TrimSpace(strings.ToLower(wallet))
	pool = strings.TrimSpace(strings.ToLower(pool))

	if chainID <= 0 || protocol == "" || wallet == "" {
		return ""
	}
	if nftTokenID > 0 {
		return NormalizePositionRef(fmt.Sprintf("%d:%s:%s:%d", chainID, protocol, wallet, nftTokenID))
	}

	lower := ""
	if tickLower != nil {
		lower = strconv.Itoa(*tickLower)
	}
	upper := ""
	if tickUpper != nil {
		upper = strconv.Itoa(*tickUpper)
	}
	if pool == "" {
		return ""
	}
	return NormalizePositionRef(fmt.Sprintf("%d:%s:%s:%s:%s:%s", chainID, protocol, wallet, pool, lower, upper))
}

func BuildPositionRefFromEvent(event *models.SmartMoneyLPEvent) string {
	if event == nil {
		return ""
	}
	nftTokenID := uint64(0)
	if event.NftTokenID != nil {
		nftTokenID = *event.NftTokenID
	}
	return BuildPositionRef(
		event.ChainID,
		event.Protocol,
		event.WalletAddress,
		nftTokenID,
		event.PoolAddress,
		event.TickLower,
		event.TickUpper,
	)
}

func BuildPositionRefFromPosition(pos *models.SmartMoneyLPPosition) string {
	if pos == nil {
		return ""
	}
	return BuildPositionRef(
		pos.ChainID,
		pos.Protocol,
		pos.WalletAddress,
		pos.NftTokenID,
		pos.PoolAddress,
		pos.TickLower,
		pos.TickUpper,
	)
}

func BuildPositionRefFromActive(pos *models.SmartMoneyActivePosition) string {
	if pos == nil {
		return ""
	}
	return BuildPositionRef(
		pos.ChainID,
		pos.Protocol,
		pos.WalletAddress,
		pos.NftTokenID,
		pos.PoolAddress,
		pos.TickLower,
		pos.TickUpper,
	)
}
