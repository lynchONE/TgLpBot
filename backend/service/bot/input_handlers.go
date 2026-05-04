package bot

import (
	"TgLpBot/base/blockchain"
	"TgLpBot/base/config"
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	"TgLpBot/service/liquidity"
	"TgLpBot/service/pool"
	"TgLpBot/service/pricing"
	"TgLpBot/service/strategy"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handlePrivateKeyInput handles private key input for wallet import
func (b *Bot) handlePrivateKeyInput(message *tgbotapi.Message, user *models.User) {
	privateKey := strings.TrimSpace(message.Text)

	// Delete user's message for security
	deleteMsg := tgbotapi.NewDeleteMessage(message.Chat.ID, message.MessageID)
	b.api.Send(deleteMsg)

	// Validate private key format
	if len(privateKey) != 64 {
		b.sendMessage(message.Chat.ID, "私钥格式无效。请发送 64 位十六进制字符串。")
		return
	}

	// Store private key temporarily
	if err := database.SetUserSessionEncrypted(user.TelegramID, "temp_private_key", privateKey, 10*time.Minute); err != nil {
		b.sendMessage(message.Chat.ID, "保存会话失败，请稍后重试。")
		return
	}
	if err := database.SetUserSession(user.TelegramID, "state", "awaiting_wallet_name", 10*time.Minute); err != nil {
		b.sendMessage(message.Chat.ID, "保存会话失败，请稍后重试。")
		return
	}

	text := "请输入此钱包的名称："
	b.sendMessage(message.Chat.ID, text)
}

// handleWalletNameInput handles wallet name input
func (b *Bot) handleWalletNameInput(message *tgbotapi.Message, user *models.User) {
	walletName := strings.TrimSpace(message.Text)

	if walletName == "" {
		b.sendMessage(message.Chat.ID, "钱包名称不能为空。请重试。")
		return
	}

	// Get stored private key
	privateKey, err := database.GetUserSessionDecrypted(user.TelegramID, "temp_private_key")
	if err != nil {
		b.sendMessage(message.Chat.ID, "会话已过期。请使用 /wallet 重新开始。")
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Import wallet
	wallet, err := b.walletService.ImportWallet(user.ID, privateKey, walletName)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("导入钱包时出错：%v", err))
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Clear session
	database.ClearUserSession(user.TelegramID)

	text := fmt.Sprintf("✅ *钱包导入成功！*\n\n*地址：* `%s`\n*名称：* %s\n\n使用 /balance 查看您的钱包余额。", wallet.Address, wallet.Name)

	b.sendMessage(message.Chat.ID, text)
}

// isV4PoolId checks if the input is a V4 PoolId (64 hex chars / 32 bytes)
func isV4PoolId(text string) bool {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		text = text[2:]
	}
	if len(text) != 64 {
		return false
	}
	for _, c := range text {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// handlePoolAddress handles pool address input for creating new position
func (b *Bot) handlePoolAddress(message *tgbotapi.Message, user *models.User) {
	poolInput := strings.TrimSpace(message.Text)
	chainSession, _ := database.GetUserSession(user.TelegramID, sessionNewPositionChain)
	chain := config.NormalizeChain(chainSession)
	if strings.TrimSpace(chainSession) == "" {
		if cfg, err := b.configService.GetOrCreate(user.ID); err == nil && cfg != nil && !cfg.MultiChainEnabled {
			chain = config.PickEnabledChain(cfg.DefaultChain)
			_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
		} else {
			chains := enabledChains()
			if len(chains) == 1 {
				chain = config.NormalizeChain(chains[0])
				_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
			} else {
				_ = database.SetUserSession(user.TelegramID, sessionPendingPoolInput, poolInput, 30*time.Minute)
				_ = database.SetUserSession(user.TelegramID, "state", sessionNewPositionState, 30*time.Minute)
				b.sendMessageWithKeyboard(
					message.Chat.ID,
					"📊 *创建新仓位*\n\n已检测到池子地址/PoolId，请先选择链：",
					newPositionChainKeyboard(chains),
				)
				return
			}
		}
	}
	stableSym, _, _ := stableSymbolForChain(chain)

	// Check if user has a wallet first
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "⚠️ 您还没有钱包。请先使用 /wallet 导入一个钱包。")
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Check if it's a V4 PoolId (64 hex chars)
	if isV4PoolId(poolInput) {
		if chain != "bsc" {
			b.sendMessage(message.Chat.ID, "❌ V4 PoolId 当前仅支持 BSC 链，请切换到 BSC 或输入 V3 池子地址。")
			return
		}

		b.sendMessage(message.Chat.ID, "⏳ 正在查询 Uniswap V4 池子信息...")

		poolInfo, err := b.poolService.GetPoolInfoForVersionCached(chain, "v4", poolInput)
		if err != nil {
			b.sendMessage(
				message.Chat.ID,
				fmt.Sprintf("❌ 查询池子信息失败（chain=%s, pool=%s）：%v\n\n请确认地址、链和协议版本是否正确。", chain, poolInput, err),
			)
			database.ClearUserSession(user.TelegramID)
			return
		}

		// Store pool info in session
		_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
		database.SetUserSession(user.TelegramID, "pool_address", poolInput, 30*time.Minute)
		// V4 does not have a per-pool contract address; we query state via PoolManager.
		// Store PoolManager address for later tick queries if configured.
		if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			database.SetUserSession(user.TelegramID, "pool_contract_address", config.AppConfig.UniswapV4PoolManagerAddress, 30*time.Minute)
		} else {
			database.SetUserSession(user.TelegramID, "pool_contract_address", "", 30*time.Minute)
		}
		database.SetUserSession(user.TelegramID, "pool_version", "v4", 30*time.Minute)
		database.SetUserSession(user.TelegramID, "pool_exchange", poolInfo.Exchange, 30*time.Minute)
		database.SetUserSession(user.TelegramID, "pool_token0", poolInfo.Token0Symbol, 30*time.Minute)
		database.SetUserSession(user.TelegramID, "pool_token1", poolInfo.Token1Symbol, 30*time.Minute)
		database.SetUserSession(user.TelegramID, "pool_fee", fmt.Sprintf("%d", poolInfo.Fee), 30*time.Minute)

		// Save Token Addresses & Hooks (for V4 PoolKey reconstruction)
		database.SetUserSession(user.TelegramID, "pool_token0_address", poolInfo.Token0, 30*time.Minute)
		database.SetUserSession(user.TelegramID, "pool_token1_address", poolInfo.Token1, 30*time.Minute)
		hooksAddr := strings.TrimSpace(poolInfo.HooksAddress)
		if !common.IsHexAddress(hooksAddr) {
			hooksAddr = "0x0000000000000000000000000000000000000000"
		}
		database.SetUserSession(user.TelegramID, "pool_hooks_address", hooksAddr, 30*time.Minute)

		tickSpacing := poolInfo.TickSpacing
		database.SetUserSession(user.TelegramID, "pool_tick_spacing", fmt.Sprintf("%d", tickSpacing), 30*time.Minute)
		database.SetUserSession(user.TelegramID, "state", "awaiting_tick_range", 30*time.Minute)

		// Use selected wallet (multi-wallet mode) or default wallet.
		selectedWallet, werr := b.ensureNewPositionWalletSession(user.ID, user.TelegramID)
		addr := ""
		if werr == nil && selectedWallet != nil {
			addr = selectedWallet.Address
		} else if len(wallets) > 0 {
			addr = wallets[0].Address
			for _, w := range wallets {
				if w.IsDefault {
					addr = w.Address
					break
				}
			}
		}
		balanceText := b.getPoolInfoWalletBalanceText(chain, addr)

		// Display pool information
		text := fmt.Sprintf(`📊 *Uniswap V4 池子信息*

🏦 *交易所：* %s
💱 *交易对：* %s/%s
💵 *手续费：* %.4f%%
🔗 *PoolId：* %s...%s
%s
📝 *下一步：* 请输入百分比范围和投入金额

*格式选项：*
1️⃣ 使用百分比范围: '金额 百分比'
   例如: '100 5' (表示投入 100 %s，当前价格 ±5%%)
2️⃣ 使用上下不对称百分比: '金额 下百分比 上百分比'
   例如: '100 1 3' (表示投入 100 %s，当前价格下方 1%%、上方 3%%)
3️⃣ 可选滑点: 末尾追加 's=滑点'
   例如: '100 5 s=0.5' 或 '100 1 3 s=0.5' (不填则使用全局滑点)

💡 提示：百分比范围会自动换算成 tick 范围

发送 /cancel 取消此操作。`,
			poolInfo.Exchange,
			poolInfo.Token0Symbol,
			poolInfo.Token1Symbol,
			float64(poolInfo.Fee)/10000,
			poolInput[:10],
			poolInput[len(poolInput)-8:],
			balanceText,
			stableSym,
			stableSym,
		)

		b.sendMessage(message.Chat.ID, text)
		return
	}

	// V3 pool address validation
	if !common.IsHexAddress(poolInput) {
		b.sendMessage(message.Chat.ID, "❌ 无效的池子标识符。\n\n请发送：\n• V3 池子地址（40位十六进制）\n• V4 PoolId（64位十六进制）")
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Query V3 pool information
	b.sendMessage(message.Chat.ID, "⏳ 正在查询 V3 池子信息...")

	poolInfo, err := b.poolService.GetPoolInfoForVersionCached(chain, "v3", poolInput)
	if err != nil {
		b.sendMessage(
			message.Chat.ID,
			fmt.Sprintf("❌ 查询池子信息失败（chain=%s, pool=%s）：%v\n\n请确认地址、链和协议版本是否正确。", chain, poolInput, err),
		)
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Store pool info in session
	_ = database.SetUserSession(user.TelegramID, sessionNewPositionChain, chain, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_address", poolInput, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_contract_address", poolInfo.Address, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_version", "v3", 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_exchange", poolInfo.Exchange, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_token0", poolInfo.Token0Symbol, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_token1", poolInfo.Token1Symbol, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_token0_address", poolInfo.Token0, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_token1_address", poolInfo.Token1, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_hooks_address", "0x0000000000000000000000000000000000000000", 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_fee", fmt.Sprintf("%d", poolInfo.Fee), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_tick_spacing", fmt.Sprintf("%d", poolInfo.TickSpacing), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_tick_range", 30*time.Minute)

	// Get wallet balance info
	var balanceText string
	if len(wallets) > 0 {
		selectedWallet, werr := b.ensureNewPositionWalletSession(user.ID, user.TelegramID)
		addr := ""
		if werr == nil && selectedWallet != nil {
			addr = selectedWallet.Address
		} else {
			addr = wallets[0].Address
			for _, w := range wallets {
				if w.IsDefault {
					addr = w.Address
					break
				}
			}
		}
		balanceText = b.getPoolInfoWalletBalanceText(chain, addr)
	}

	// Display pool information
	text := fmt.Sprintf(`📊 *池子信息*

🏦 *交易所：* %s
💱 *交易对：* %s/%s
💵 *手续费：* %.4f%%
%s
📝 *下一步：* 请输入百分比范围和投入金额

*格式选项：*
1️⃣ 使用百分比范围: '金额 百分比'
   例如: '100 5' (表示投入 100 %s，当前价格 ±5%%)
2️⃣ 使用上下不对称百分比: '金额 下百分比 上百分比'
   例如: '100 1 3' (表示投入 100 %s，当前价格下方 1%%、上方 3%%)
3️⃣ 可选滑点: 末尾追加 's=滑点'
   例如: '100 5 s=0.5' 或 '100 1 3 s=0.5' (不填则使用全局滑点)

💡 提示：百分比范围会自动换算成 tick 范围

发送 /cancel 取消此操作。`,
		poolInfo.Exchange,
		poolInfo.Token0Symbol,
		poolInfo.Token1Symbol,
		float64(poolInfo.Fee)/10000,
		balanceText,
		stableSym,
		stableSym,
	)

	b.sendMessage(message.Chat.ID, text)
}

// handleTickRange handles tick range input
func (b *Bot) handleTickRange(message *tgbotapi.Message, user *models.User) {
	input := strings.TrimSpace(message.Text)
	chain, chainErr := resolveNewPositionChain(user.ID, user.TelegramID)
	if chainErr != nil {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "会话已过期或未选择链。请重新使用 /newposition 并先选择链。")
		return
	}
	stableSym, _, _ := stableSymbolForChain(chain)

	// Resolve wallet for this new position (selected wallet in multi-wallet mode, otherwise default).
	selectedWallet, werr := b.ensureNewPositionWalletSession(user.ID, user.TelegramID)
	if werr != nil || selectedWallet == nil {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "⚠️ 您还没有钱包，请先用 /wallet 导入。")
		return
	}

	// Expect:
	// - "amount percentage" (symmetric)
	// - "amount lowerPct upperPct" (asymmetric)
	// Optional: append slippage override, e.g. "100 5 s=0.5" or "100 1 3 s=0.5"
	fields := strings.Fields(input)

	parseSlippageToken := func(token string) (float64, bool, error) {
		raw := strings.TrimSpace(token)
		if raw == "" {
			return 0, false, nil
		}

		lower := strings.ToLower(raw)
		valueStr := ""
		switch {
		case strings.HasPrefix(lower, "s=") || strings.HasPrefix(lower, "s:"):
			valueStr = raw[2:]
		case strings.HasPrefix(lower, "slip="):
			valueStr = raw[5:]
		case strings.HasPrefix(lower, "slippage="):
			valueStr = raw[len("slippage="):]
		case strings.HasPrefix(lower, "滑点="):
			valueStr = raw[len("滑点="):]
		case strings.HasPrefix(lower, "s") && len(raw) > 1:
			// s0.5
			valueStr = raw[1:]
		default:
			return 0, false, nil
		}

		valueStr = strings.TrimSpace(strings.TrimSuffix(valueStr, "%"))
		v, err := strconv.ParseFloat(valueStr, 64)
		if err != nil || v < 0 || v > 100 {
			return 0, true, fmt.Errorf("invalid slippage")
		}
		return v, true, nil
	}

	var slippageOverride *float64
	filtered := make([]string, 0, len(fields))
	for _, f := range fields {
		if v, ok, err := parseSlippageToken(f); ok {
			if err != nil {
				b.sendMessage(message.Chat.ID, "滑点无效。请输入 0-100 之间的滑点百分比，例如：`s=0.5` 表示 0.5%（不填则使用全局滑点）。")
				return
			}
			slippageOverride = &v
			continue
		}
		filtered = append(filtered, f)
	}
	fields = filtered

	var amount float64
	var stableLowerPctReq float64
	var stableUpperPctReq float64
	symmetric := false

	switch len(fields) {
	case 2:
		// "amount percentage"
		a, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || a <= 0 {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("金额无效。请输入正数。\n\n例如：`100 5` 表示投入 100 %s，当前价格 ±5%%。", stableSym))
			return
		}

		percentStr := strings.TrimSuffix(fields[1], "%")
		pct, err := strconv.ParseFloat(percentStr, 64)
		if err != nil || pct <= 0 || pct >= 100 {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("百分比无效。请输入 0 到 100 之间的数字（不含 100）。\n\n例如：`100 5` 表示投入 100 %s，当前价格 ±5%%。", stableSym))
			return
		}

		amount = a
		stableLowerPctReq = pct
		stableUpperPctReq = pct
		symmetric = true
	case 3:
		// "amount lowerPct upperPct"
		a, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || a <= 0 {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("金额无效。请输入正数。\n\n例如：`100 1 3`（投入 100 %s，当前价格下方 1%%、上方 3%%）", stableSym))
			return
		}
		lowStr := strings.TrimSuffix(fields[1], "%")
		upStr := strings.TrimSuffix(fields[2], "%")
		lowPct, err1 := strconv.ParseFloat(lowStr, 64)
		upPct, err2 := strconv.ParseFloat(upStr, 64)
		if err1 != nil || err2 != nil || lowPct <= 0 || upPct <= 0 || lowPct >= 100 || upPct >= 100 {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("百分比无效。请输入 0 到 100 之间的数字（不含 100）。\n\n例如：`100 1 3`（投入 100 %s，当前价格下方 1%%、上方 3%%）", stableSym))
			return
		}

		amount = a
		stableLowerPctReq = lowPct
		stableUpperPctReq = upPct
	default:
		b.sendMessage(message.Chat.ID, fmt.Sprintf("格式无效。请使用：\n1) `金额 百分比`（例如：`100 5` 表示投入 100 %s，当前价格 ±5%%）\n2) `金额 下百分比 上百分比`（例如：`100 1 3` 表示投入 100 %s，当前价格下方 1%%、上方 3%%）\n可选滑点：末尾追加 `s=0.5`", stableSym, stableSym))
		return
	}

	// Validate amount against selected wallet stable balance when possible
	client, _, _ := blockchain.GetEVMClient(chain)
	if client != nil {
		addr := common.HexToAddress(selectedWallet.Address)
		stableSym, stableDecimals, stableAddrStr := stableSymbolForChain(chain)
		if !common.IsHexAddress(stableAddrStr) && chain == "bsc" {
			stableAddrStr = "0x55d398326f99059fF775485246999027B3197955"
		}
		if common.IsHexAddress(stableAddrStr) {
			stableAddr := common.HexToAddress(stableAddrStr)
			stableBal, err := blockchain.GetTokenBalanceWithClient(client, stableAddr, addr)
			if err == nil {
				stableFloat := new(big.Float).SetInt(stableBal)
				divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(stableDecimals)), nil))
				stableFloat.Quo(stableFloat, divisor)
				stableAvailable, _ := stableFloat.Float64()
				if amount-stableAvailable > 1e-9 {
					b.sendMessage(message.Chat.ID, fmt.Sprintf("余额不足。当前钱包 %s 余额：%.2f\n\n请输入不超过余额的金额。", stableSym, stableAvailable))
					return
				}
			}
		}
	}

	// Read pool tick spacing
	tickSpacingStr, err := database.GetUserSession(user.TelegramID, "pool_tick_spacing")
	if err != nil || tickSpacingStr == "" {
		b.sendMessage(message.Chat.ID, "会话已过期。请重新输入池子地址。")
		database.ClearUserSession(user.TelegramID)
		return
	}
	tickSpacing, err := strconv.Atoi(tickSpacingStr)
	if err != nil || tickSpacing <= 0 {
		b.sendMessage(message.Chat.ID, "无法解析 tick spacing。请重新输入池子地址。")
		database.ClearUserSession(user.TelegramID)
		return
	}

	poolVersion, _ := database.GetUserSession(user.TelegramID, "pool_version")
	poolID, _ := database.GetUserSession(user.TelegramID, "pool_address")
	token0Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token0")
	token1Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token1")
	token0Addr, _ := database.GetUserSession(user.TelegramID, "pool_token0_address")
	token1Addr, _ := database.GetUserSession(user.TelegramID, "pool_token1_address")

	// Prepare a minimal task context for stable-side detection and stable/tick percentage conversion.
	tmpTask := &models.StrategyTask{
		Chain:         chain,
		PoolId:        poolID,
		PoolVersion:   poolVersion,
		Token0Symbol:  token0Symbol,
		Token1Symbol:  token1Symbol,
		Token0Address: token0Addr,
		Token1Address: token1Addr,
	}
	tickLowerPctReq, tickUpperPctReq := pricing.TickPercentagesFromStablePercentages(tmpTask, stableLowerPctReq, stableUpperPctReq)
	if tickLowerPctReq <= 0 || tickUpperPctReq <= 0 {
		b.sendMessage(message.Chat.ID, "百分比无效。请检查输入范围。\n\n例如：`5 100` 或 `100 1 3`")
		return
	}

	// Fetch current tick from chain (V3 via pool.slot0; V4 via PoolManager.getSlot0/slot0)
	var currentTick int
	switch strings.ToLower(strings.TrimSpace(poolVersion)) {
	case "v4":
		if config.AppConfig == nil || !common.IsHexAddress(config.AppConfig.UniswapV4PoolManagerAddress) {
			b.sendMessage(message.Chat.ID, "未配置 Uniswap V4 PoolManager 地址，无法查询当前 tick 并按百分比换算。\n\n请在 `.env` 中设置 `UNISWAP_V4_POOL_MANAGER_ADDRESS` 后重试。")
			return
		}
		if !common.IsHexAddress(config.AppConfig.UniswapV4StateViewAddress) {
			b.sendMessage(message.Chat.ID, "未配置 Uniswap V4 StateView 地址，无法查询当前 tick。\n\n请在 `.env` 中设置 `UNISWAP_V4_STATE_VIEW_ADDRESS` 后重试。")
			return
		}
		poolManager := common.HexToAddress(config.AppConfig.UniswapV4PoolManagerAddress)
		stateView := common.HexToAddress(config.AppConfig.UniswapV4StateViewAddress)
		// Use StateView directly (PoolManager.slot0 is not supported)
		currentTick, err = blockchain.GetUniswapV4PoolCurrentTickViaStateView(stateView, poolManager, poolID)
		if err != nil {
			if strings.Contains(err.Error(), "execution reverted") {
				b.sendMessage(message.Chat.ID, "获取 V4 当前 tick 失败：StateView 调用被回滚（execution reverted）。\n\n常见原因：\n1) StateView 地址配错\n2) PoolId 不存在/未初始化\n\n请检查 `.env` 中的 `UNISWAP_V4_STATE_VIEW_ADDRESS` 配置。")
				return
			}
			b.sendMessage(message.Chat.ID, fmt.Sprintf("获取 V4 当前 tick 失败，无法按百分比计算 tick 范围：%v", err))
			return
		}
	default:
		poolContractAddrStr, _ := database.GetUserSession(user.TelegramID, "pool_contract_address")
		if poolContractAddrStr == "" {
			poolContractAddrStr, _ = database.GetUserSession(user.TelegramID, "pool_address")
		}
		if !common.IsHexAddress(poolContractAddrStr) {
			b.sendMessage(message.Chat.ID, "当前池子不支持按百分比自动换算 tick（缺少可调用的池子合约地址）。请更换 V3 类池子地址后重试。")
			return
		}

		client, _, cerr := blockchain.GetEVMClient(chain)
		if cerr != nil {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("连接链节点失败，无法按百分比计算 tick 范围：%v", cerr))
			return
		}
		currentTick, err = blockchain.GetV3PoolCurrentTickWithClient(client, common.HexToAddress(poolContractAddrStr))
		if err != nil {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("获取当前 tick 失败，无法按百分比计算 tick 范围：%v", err))
			return
		}
	}

	tc := pool.NewTickCalculator()
	// Use best-fit rounding to minimize distortion caused by tickSpacing quantization,
	// especially on higher fee tiers (e.g. 1% pools have large tickSpacing).
	tickLower, tickUpper := tc.CalculateTickFromPercentagesBestFit(currentTick, tickLowerPctReq, tickUpperPctReq, tickSpacing)
	tickLowerPctEff, tickUpperPctEff := tc.CalculatePercentagesFromTicks(currentTick, tickLower, tickUpper)
	effectivePct := (tickLowerPctEff + tickUpperPctEff) / 2.0
	stableLowerPctEff, stableUpperPctEff := pricing.StablePercentagesFromTickPercentages(tmpTask, tickLowerPctEff, tickUpperPctEff)

	// Store tick range and amount
	database.SetUserSession(user.TelegramID, "tick_lower", strconv.Itoa(tickLower), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "tick_upper", strconv.Itoa(tickUpper), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "position_amount", fmt.Sprintf("%.8f", amount), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "tick_percentage", fmt.Sprintf("%.8f", effectivePct), 30*time.Minute)
	if tickLowerPctEff > 0 && tickUpperPctEff > 0 {
		database.SetUserSession(user.TelegramID, "tick_lower_percentage", fmt.Sprintf("%.8f", tickLowerPctEff), 30*time.Minute)
		database.SetUserSession(user.TelegramID, "tick_upper_percentage", fmt.Sprintf("%.8f", tickUpperPctEff), 30*time.Minute)
	}
	if slippageOverride != nil {
		database.SetUserSession(user.TelegramID, "position_slippage", fmt.Sprintf("%.8f", *slippageOverride), 30*time.Minute)
	} else {
		_ = database.DeleteUserSession(user.TelegramID, "position_slippage")
	}

	// 直接创建任务，不需要确认
	var rangeLine string
	switch {
	case symmetric:
		requested := stableLowerPctReq
		avgEff := (stableLowerPctEff + stableUpperPctEff) / 2.0

		rangeLine = fmt.Sprintf("📈 百分比范围：±%.6f%%", requested)
		if stableLowerPctEff > 0 && stableUpperPctEff > 0 {
			asymmetricEff := math.Abs(stableLowerPctEff-stableUpperPctEff) >= 0.0001
			drift := math.Abs(avgEff-requested) >= 0.0001

			if asymmetricEff {
				rangeLine = fmt.Sprintf("📈 百分比范围：输入 ±%.6f%%，实际 下 %.6f%% / 上 %.6f%% (受费率/格子影响)", requested, stableLowerPctEff, stableUpperPctEff)
			} else if drift {
				rangeLine = fmt.Sprintf("📈 百分比范围：输入 ±%.6f%%，实际 ±%.6f%% (受费率/格子影响)", requested, avgEff)
			} else {
				rangeLine = fmt.Sprintf("📈 百分比范围：±%.6f%%", avgEff)
			}
		}
	default:
		rangeLine = fmt.Sprintf("📈 百分比范围：下 %.6f%% / 上 %.6f%%", stableLowerPctReq, stableUpperPctReq)
		if stableLowerPctEff > 0 && stableUpperPctEff > 0 {
			drift := math.Abs(stableLowerPctEff-stableLowerPctReq) >= 0.0001 || math.Abs(stableUpperPctEff-stableUpperPctReq) >= 0.0001
			if drift {
				rangeLine = fmt.Sprintf("📈 百分比范围：输入 下 %.6f%% / 上 %.6f%%，实际 下 %.6f%% / 上 %.6f%% (受费率/格子影响)", stableLowerPctReq, stableUpperPctReq, stableLowerPctEff, stableUpperPctEff)
			} else {
				rangeLine = fmt.Sprintf("📈 百分比范围：下 %.6f%% / 上 %.6f%%", stableLowerPctEff, stableUpperPctEff)
			}
		}
	}

	b.sendMessage(message.Chat.ID, fmt.Sprintf(`📊 *任务参数*

%s
🎯 当前 Tick：%d
📊 Tick 范围：%d 到 %d
💰 投入金额：%.2f %s

⏳ 正在创建任务并开仓...`, rangeLine, currentTick, tickLower, tickUpper, amount, stableSym))

	// 调用创建任务逻辑
	b.createPositionTask(message.Chat.ID, user)
}

// createPositionTask 创建任务并开仓
func (b *Bot) createPositionTask(chatID int64, user *models.User) {
	chain, chainErr := resolveNewPositionChain(user.ID, user.TelegramID)
	if chainErr != nil {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(chatID, "会话已过期或未选择链。请重新使用 /newposition 并先选择链。")
		return
	}

	// Get stored data
	poolAddress, _ := database.GetUserSession(user.TelegramID, "pool_address")
	poolVersion, _ := database.GetUserSession(user.TelegramID, "pool_version")
	poolExchange, _ := database.GetUserSession(user.TelegramID, "pool_exchange")
	token0Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token0")
	token1Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token1")
	feeStr, _ := database.GetUserSession(user.TelegramID, "pool_fee")
	tickSpacingStr, _ := database.GetUserSession(user.TelegramID, "pool_tick_spacing")
	rangePctStr, _ := database.GetUserSession(user.TelegramID, "tick_percentage")
	rangeLowerPctStr, _ := database.GetUserSession(user.TelegramID, "tick_lower_percentage")
	rangeUpperPctStr, _ := database.GetUserSession(user.TelegramID, "tick_upper_percentage")
	tickLowerStr, _ := database.GetUserSession(user.TelegramID, "tick_lower")
	tickUpperStr, _ := database.GetUserSession(user.TelegramID, "tick_upper")
	amountStr, _ := database.GetUserSession(user.TelegramID, "position_amount")

	token0Addr, _ := database.GetUserSession(user.TelegramID, "pool_token0_address")
	token1Addr, _ := database.GetUserSession(user.TelegramID, "pool_token1_address")
	hooksAddr, _ := database.GetUserSession(user.TelegramID, "pool_hooks_address")
	if hooksAddr == "" {
		hooksAddr = "0x0000000000000000000000000000000000000000"
	}

	tickLower, _ := strconv.Atoi(tickLowerStr)
	tickUpper, _ := strconv.Atoi(tickUpperStr)
	amount, _ := strconv.ParseFloat(amountStr, 64)
	fee, _ := strconv.Atoi(feeStr)
	tickSpacing, _ := strconv.Atoi(tickSpacingStr)
	rangePct, _ := strconv.ParseFloat(rangePctStr, 64)
	rangeLowerPct, _ := strconv.ParseFloat(rangeLowerPctStr, 64)
	rangeUpperPct, _ := strconv.ParseFloat(rangeUpperPctStr, 64)
	if rangeLowerPct <= 0 || rangeUpperPct <= 0 {
		rangeLowerPct = 0
		rangeUpperPct = 0
	}

	cfg, cfgErr := b.configService.GetOrCreate(user.ID)
	if cfgErr != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 获取全局配置失败: %v", cfgErr))
		return
	}

	slippage := cfg.SlippageTolerance
	if slippageStr, err := database.GetUserSession(user.TelegramID, "position_slippage"); err == nil {
		if v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(slippageStr, "%")), 64); err == nil && v >= 0 && v <= 100 {
			slippage = v
		}
	}

	// Resolve wallet selection (or default wallet).
	selectedWallet, werr := b.resolveNewPositionWallet(user.ID, user.TelegramID)
	if werr != nil || selectedWallet == nil {
		b.sendMessage(chatID, "⚠️ 您还没有钱包，请先用 /wallet 导入。")
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Create Strategy Task
	task := &models.StrategyTask{
		UserID:               user.ID,
		Chain:                chain,
		PoolId:               poolAddress,
		PoolVersion:          poolVersion,
		Exchange:             poolExchange,
		WalletID:             selectedWallet.ID,
		WalletAddress:        selectedWallet.Address,
		Token0Symbol:         token0Symbol,
		Token1Symbol:         token1Symbol,
		Token0Address:        token0Addr,
		Token1Address:        token1Addr,
		HooksAddress:         hooksAddr,
		Fee:                  fee,
		TickSpacing:          tickSpacing,
		TickLower:            tickLower,
		TickUpper:            tickUpper,
		RangePercentage:      rangePct,
		RangeLowerPercentage: rangeLowerPct,
		RangeUpperPercentage: rangeUpperPct,
		AmountUSDT:           amount,
		CurrentLiquidity:     "0", // Will be updated after zap in
		ReopenDelaySeconds:   strategy.NormalizeRebalanceTimeout(cfg.RebalanceTimeout),
		SlippageTolerance:    slippage,
		AutoReinvest:         cfg.AutoReinvest,
		RebalanceEnabled:     false,
		OutOfRangeMode:       string(models.StrategyOutOfRangeModeExitAll),
		Paused:               true,
		Status:               models.StrategyStatusRunning,
		LastCheckTime:        time.Now(),
	}
	now := time.Now()
	task.PausedAt = &now

	if err := b.strategyService.CreateTask(task); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 创建任务失败: %v", err))
		return
	}

	b.sendMessage(chatID, "⛓️ 任务已创建，正在准备开仓...")

	enterRes, err := b.liquidityService.EnterTaskFromUSDT(user.ID, task)
	if err != nil {
		var swapErr *liquidity.EntrySwapRequiredError
		if errors.As(err, &swapErr) {
			b.promptEntrySwap(chatID, task, swapErr.TokenSymbol)
			_ = database.ClearUserSession(user.TelegramID)
			return
		}
		_ = database.DB.Model(task).Updates(map[string]interface{}{
			"status":        models.StrategyStatusError,
			"error_message": fmt.Sprintf("enter failed: %v", err),
		}).Error
		b.sendMessage(chatID, fmt.Sprintf("❌ 开仓失败：%v", err))
		b.sendMessageWithKeyboard(chatID, b.formatTaskCard(task), b.taskKeyboard(task))
		return
	}

	if err := b.applyEnterResult(task, enterRes); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("更新任务失败：%v", err))
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ 开仓成功！交易哈希：`%s`", enterRes.TxHash))

	// 成功后清除会话
	database.ClearUserSession(user.TelegramID)

	if msg, err := b.sendTaskCardMessage(chatID, b.formatTaskCardWithRefresh(task), b.taskKeyboardWithRefresh(task)); err == nil && msg.MessageID != 0 {
		b.startTaskAutoRefresh(chatID, msg.MessageID, task.ID, user.ID)
	}
}

// handlePositionAmount handles position amount input
func (b *Bot) handlePositionAmount(message *tgbotapi.Message, user *models.User) {
	amountStr := strings.TrimSpace(message.Text)

	// Parse amount
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		b.sendMessage(message.Chat.ID, "金额无效。请输入正数。")
		return
	}

	// Get stored data
	chain, chainErr := resolveNewPositionChain(user.ID, user.TelegramID)
	if chainErr != nil {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "会话已过期或未选择链。请重新使用 /newposition 并先选择链。")
		return
	}
	stableSym, _, _ := stableSymbolForChain(chain)
	poolAddress, _ := database.GetUserSession(user.TelegramID, "pool_address")
	tickLowerStr, _ := database.GetUserSession(user.TelegramID, "tick_lower")
	tickUpperStr, _ := database.GetUserSession(user.TelegramID, "tick_upper")

	tickLower, _ := strconv.Atoi(tickLowerStr)
	tickUpper, _ := strconv.Atoi(tickUpperStr)

	// Clear session
	database.ClearUserSession(user.TelegramID)

	// Create position and start task
	b.sendMessage(message.Chat.ID, "⏳ 正在创建仓位...")

	// TODO: Implement position creation and task启动
	text := fmt.Sprintf(`✅ *仓位创建成功！*

📊 *仓位信息：*
池子地址：`+"`%s`"+`
Tick 范围：%d 到 %d
投入金额：%.2f %s

🔄 任务已启动，正在后台运行...

使用 /positions 查看您的仓位。`,
		poolAddress,
		tickLower,
		tickUpper,
		amount,
		stableSym,
	)

	b.sendMessage(message.Chat.ID, text)
}
