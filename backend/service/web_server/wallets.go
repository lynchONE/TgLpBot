package web_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
	"TgLpBot/service/wallet"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"
)

type walletsRequest struct {
	InitData string `json:"initData"`
	Chain    string `json:"chain,omitempty"`
}

type walletBalanceRow struct {
	ID            uint   `json:"id"`
	Address       string `json:"address"`
	Name          string `json:"name,omitempty"`
	IsDefault     bool   `json:"is_default"`
	NativeBalance string `json:"native_balance"`
	StableBalance string `json:"stable_balance"`
}

type walletsResponse struct {
	OK           bool               `json:"ok"`
	Chain        string             `json:"chain"`
	NativeSymbol string             `json:"native_symbol"`
	StableSymbol string             `json:"stable_symbol"`
	Wallets      []walletBalanceRow `json:"wallets"`
}

func nativeSymbolForChainConfig(chain string, cc config.ChainConfig) string {
	w := strings.ToUpper(strings.TrimSpace(cc.WrappedNativeSymbol))
	if w != "" {
		if strings.HasPrefix(w, "W") && len(w) > 1 {
			return w[1:]
		}
		return w
	}
	switch config.NormalizeChain(chain) {
	case "base":
		return "ETH"
	default:
		return "BNB"
	}
}

func stableSymbolForChainConfig(cc config.ChainConfig) string {
	s := strings.ToUpper(strings.TrimSpace(cc.StableSymbol))
	if s == "" {
		return "USDT"
	}
	return s
}

func (s *Server) handleWallets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if config.AppConfig == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req walletsRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	initData := strings.TrimSpace(req.InitData)
	user, status, msg := authenticateTelegramWebAppUser(initData)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}

	check, status, msg, err := requireUserAccess(user.ID)
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	if status, msg := requireModulePermission(check, models.AccessModuleAssets); status != 0 {
		http.Error(w, msg, status)
		return
	}

	cfgService := userSvc.NewGlobalConfigService()
	cfg, err := cfgService.GetOrCreate(user.ID)
	if err != nil {
		http.Error(w, "failed to load config", http.StatusInternalServerError)
		return
	}

	requestedChain := strings.TrimSpace(req.Chain)
	chain := ""
	if cfg != nil && !cfg.MultiChainEnabled {
		chain = config.PickEnabledChain(cfg.DefaultChain)
	} else if requestedChain != "" {
		chain = config.NormalizeChain(requestedChain)
	} else {
		chain = config.PickEnabledChain("bsc")
	}

	cc, ok := config.AppConfig.GetChainConfig(chain)
	if !ok || strings.TrimSpace(cc.Chain) == "" {
		http.Error(w, "invalid chain", http.StatusBadRequest)
		return
	}

	walletService := wallet.NewWalletService()
	wallets, err := walletService.GetUserWallets(user.ID)
	if err != nil {
		http.Error(w, "failed to load wallets", http.StatusInternalServerError)
		return
	}
	if len(wallets) == 0 {
		http.Error(w, "no wallet found", http.StatusBadRequest)
		return
	}

	out := make([]walletBalanceRow, 0, len(wallets))
	for i := range wallets {
		ww := wallets[i]
		out = append(out, walletBalanceRow{
			ID:            ww.ID,
			Address:       strings.TrimSpace(ww.Address),
			Name:          strings.TrimSpace(ww.Name),
			IsDefault:     ww.IsDefault,
			NativeBalance: "N/A",
			StableBalance: "N/A",
		})
	}

	client, _, cerr := blockchain.GetEVMClient(chain)
	stableAddrStr := strings.TrimSpace(cc.StableAddress)
	stableAddrOk := common.IsHexAddress(stableAddrStr)
	stableAddr := common.Address{}
	if stableAddrOk {
		stableAddr = common.HexToAddress(stableAddrStr)
	}
	stableDecimals := cc.StableDecimals
	if stableDecimals <= 0 {
		stableDecimals = 18
	}

	var stableToken *blockchain.ERC20
	if client != nil && cerr == nil && stableAddrOk {
		if t, err := blockchain.NewERC20(stableAddr, client); err == nil {
			stableToken = t
		}
	}

	if client != nil && cerr == nil {
		g := new(errgroup.Group)
		g.SetLimit(4)

		for i := range out {
			i := i
			addrStr := strings.TrimSpace(out[i].Address)
			g.Go(func() error {
				if !common.IsHexAddress(addrStr) {
					return nil
				}
				addr := common.HexToAddress(addrStr)

				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()

				if bal, err := client.BalanceAt(ctx, addr, nil); err == nil && bal != nil {
					out[i].NativeBalance = fmt.Sprintf("%.6f", amountToFloat(bal.String(), 18))
				}
				if stableToken != nil {
					if bal, err := stableToken.BalanceOf(&bind.CallOpts{Context: ctx}, addr); err == nil && bal != nil {
						out[i].StableBalance = fmt.Sprintf("%.2f", amountToFloat(bal.String(), stableDecimals))
					}
				}
				return nil
			})
		}
		_ = g.Wait()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(walletsResponse{
		OK:           true,
		Chain:        chain,
		NativeSymbol: nativeSymbolForChainConfig(chain, cc),
		StableSymbol: stableSymbolForChainConfig(cc),
		Wallets:      out,
	})
}
