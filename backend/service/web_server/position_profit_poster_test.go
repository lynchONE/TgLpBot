package web_server

import (
	"testing"

	"TgLpBot/base/config"
	"TgLpBot/base/models"
	"TgLpBot/service/realtime"
)

func TestPickPosterThemeToken_PrefersNonBaseLikeToken(t *testing.T) {
	oldConfig := config.AppConfig
	config.AppConfig = &config.Config{
		Chains: map[string]config.ChainConfig{
			"bsc": {
				Chain:                "bsc",
				ChainID:              56,
				USDTAddress:          "0x55d398326f99059ff775485246999027b3197955",
				WrappedNativeAddress: "0xbb4cdb9cbd36b01bd1cbaebf2de08d9173bc095c",
			},
		},
	}
	t.Cleanup(func() {
		config.AppConfig = oldConfig
	})

	task := &models.StrategyTask{
		Chain:         "bsc",
		Token0Address: "0x55d398326f99059ff775485246999027b3197955",
		Token0Symbol:  "USDT",
		Token1Address: "0x1111111111111111111111111111111111111111",
		Token1Symbol:  "TEST",
	}
	position := &realtime.RealtimePosition{
		Chain: "bsc",
		TokenRows: []realtime.RealtimeTokenRow{
			{Address: "0x55d398326f99059ff775485246999027b3197955", Symbol: "USDT"},
			{Address: "0x1111111111111111111111111111111111111111", Symbol: "TEST"},
		},
	}

	got := pickPosterThemeToken("bsc", task, position)
	if got.address != "0x1111111111111111111111111111111111111111" || got.symbol != "TEST" {
		t.Fatalf("expected non-base token, got %+v", got)
	}
}
