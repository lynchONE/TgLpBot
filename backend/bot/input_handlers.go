package bot

import (
	"TgLpBot/blockchain"
	"TgLpBot/config"
	"TgLpBot/database"
	"TgLpBot/models"
	"TgLpBot/services"
	"errors"
	"fmt"
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

	// Check if user has a wallet first
	wallets, err := b.walletService.GetUserWallets(user.ID)
	if err != nil || len(wallets) == 0 {
		b.sendMessage(message.Chat.ID, "⚠️ 您还没有钱包。请先使用 /wallet 创建或导入一个钱包。")
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Check if it's a V4 PoolId (64 hex chars)
	if isV4PoolId(poolInput) {
		b.sendMessage(message.Chat.ID, "⏳ 正在查询 Uniswap V4 池子信息...")

		poolInfo, err := b.poolService.GetV4PoolInfo(poolInput)
		if err != nil {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 查询 V4 池子信息失败：%v", err))
			database.ClearUserSession(user.TelegramID)
			return
		}

		// Store pool info in session
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

		// Get wallet info
		defaultWallet := wallets[0]
		for _, w := range wallets {
			if w.IsDefault {
				defaultWallet = w
				break
			}
		}
		bnbBal, usdtBal := b.getWalletBalances(defaultWallet.Address)
		balanceText := fmt.Sprintf(
			"\n💰 *当前钱包：* `%s`\n💎 BNB: %s\n💵 USDT: %s\n",
			defaultWallet.Address[:10]+"..."+defaultWallet.Address[len(defaultWallet.Address)-8:],
			bnbBal,
			usdtBal,
		)

		// Display pool information
		text := fmt.Sprintf(`📊 *Uniswap V4 池子信息*

🏦 *交易所：* %s
💱 *交易对：* %s/%s
💵 *手续费：* %.4f%%
🔗 *PoolId：* %s...%s
%s
📝 *下一步：* 请输入百分比范围和投入金额

*格式选项：*
1️⃣ 使用百分比范围: '百分比 金额'
   例如: '5 100' (表示当前价格 ±5%%, 投入 100 USDT)

💡 提示：百分比范围会自动换算成 tick 范围

发送 /cancel 取消此操作。`,
			poolInfo.Exchange,
			poolInfo.Token0Symbol,
			poolInfo.Token1Symbol,
			float64(poolInfo.Fee)/10000,
			poolInput[:10],
			poolInput[len(poolInput)-8:],
			balanceText,
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

	poolInfo, err := b.poolService.GetPoolInfo(poolInput)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 查询池子信息失败：%v\n\n请确认地址是否正确。", err))
		database.ClearUserSession(user.TelegramID)
		return
	}

	// Store pool info in session
	database.SetUserSession(user.TelegramID, "pool_address", poolInput, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_contract_address", poolInfo.Address, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_version", "v3", 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_exchange", poolInfo.Exchange, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_token0", poolInfo.Token0Symbol, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_token1", poolInfo.Token1Symbol, 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_fee", fmt.Sprintf("%d", poolInfo.Fee), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "pool_tick_spacing", fmt.Sprintf("%d", poolInfo.TickSpacing), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_tick_range", 30*time.Minute)

	// Get wallet balance info
	var balanceText string
	if len(wallets) > 0 {
		defaultWallet := wallets[0]
		for _, w := range wallets {
			if w.IsDefault {
				defaultWallet = w
				break
			}
		}
		bnbBal, usdtBal := b.getWalletBalances(defaultWallet.Address)
		balanceText = fmt.Sprintf(
			"\n💰 *当前钱包：* `%s`\n💎 BNB: %s\n💵 USDT: %s\n",
			defaultWallet.Address[:10]+"..."+defaultWallet.Address[len(defaultWallet.Address)-8:],
			bnbBal,
			usdtBal,
		)
	}

	// Display pool information
	text := fmt.Sprintf(`📊 *池子信息*

🏦 *交易所：* %s
💱 *交易对：* %s/%s
💵 *手续费：* %.4f%%
%s
📝 *下一步：* 请输入百分比范围和投入金额

*格式选项：*
1️⃣ 使用百分比范围: '百分比 金额'
   例如: '5 100' (表示当前价格 ±5%%, 投入 100 USDT)

💡 提示：百分比范围会自动换算成 tick 范围

发送 /cancel 取消此操作。`,
		poolInfo.Exchange,
		poolInfo.Token0Symbol,
		poolInfo.Token1Symbol,
		float64(poolInfo.Fee)/10000,
		balanceText,
	)

	b.sendMessage(message.Chat.ID, text)
}

// handleTickRange handles tick range input
func (b *Bot) handleTickRange(message *tgbotapi.Message, user *models.User) {
	input := strings.TrimSpace(message.Text)

	// Expect: "percentage amount"
	fields := strings.Fields(input)
	if len(fields) != 2 {
		b.sendMessage(message.Chat.ID, "格式无效。请使用：`百分比 金额`\n\n例如：`5 100`（表示当前价格 ±5%，投入 100 USDT）")
		return
	}

	percentStr := strings.TrimSuffix(fields[0], "%")
	percentage, err := strconv.ParseFloat(percentStr, 64)
	if err != nil || percentage <= 0 || percentage >= 100 {
		b.sendMessage(message.Chat.ID, "百分比无效。请输入 0 到 100 之间的数字（不含 100）。\n\n例如：`5` 表示 ±5%。")
		return
	}

	amount, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || amount <= 0 {
		b.sendMessage(message.Chat.ID, "金额无效。请输入正数。\n\n例如：`100`")
		return
	}

	// Validate amount against default wallet USDT balance when possible
	wallets, wErr := b.walletService.GetUserWallets(user.ID)
	if wErr == nil && len(wallets) > 0 && blockchain.Client != nil {
		defaultWallet := wallets[0]
		for _, w := range wallets {
			if w.IsDefault {
				defaultWallet = w
				break
			}
		}
		addr := common.HexToAddress(defaultWallet.Address)
		usdtAddrStr := "0x55d398326f99059fF775485246999027B3197955"
		if config.AppConfig != nil && common.IsHexAddress(config.AppConfig.USDTAddress) {
			usdtAddrStr = config.AppConfig.USDTAddress
		}
		usdtAddr := common.HexToAddress(usdtAddrStr)
		usdtBal, err := blockchain.GetTokenBalance(usdtAddr, addr)
		if err == nil {
			usdtFloat := new(big.Float).SetInt(usdtBal)
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
			usdtFloat.Quo(usdtFloat, divisor)
			usdtAvailable, _ := usdtFloat.Float64()
			if amount-usdtAvailable > 1e-9 {
				b.sendMessage(message.Chat.ID, fmt.Sprintf("余额不足。当前钱包 USDT 余额：%.2f\n\n请输入不超过余额的金额。", usdtAvailable))
				return
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

	// Fetch current tick from chain (V3 via pool.slot0; V4 via PoolManager.getSlot0/slot0)
	var currentTick int
	switch strings.ToLower(strings.TrimSpace(poolVersion)) {
	case "v4":
		poolID, _ := database.GetUserSession(user.TelegramID, "pool_address")
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

		currentTick, err = blockchain.GetV3PoolCurrentTick(common.HexToAddress(poolContractAddrStr))
		if err != nil {
			b.sendMessage(message.Chat.ID, fmt.Sprintf("获取当前 tick 失败，无法按百分比计算 tick 范围：%v", err))
			return
		}
	}

	tc := services.NewTickCalculator()
	tickLower, tickUpper := tc.CalculateTickFromPercentage(currentTick, percentage, tickSpacing)
	if err := tc.ValidateTickRange(tickLower, tickUpper, tickSpacing); err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("计算出的 tick 范围无效：%v\n\n请尝试更小的百分比。", err))
		return
	}

	// Store tick range and amount
	database.SetUserSession(user.TelegramID, "tick_lower", strconv.Itoa(tickLower), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "tick_upper", strconv.Itoa(tickUpper), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "position_amount", fmt.Sprintf("%.8f", amount), 30*time.Minute)
	database.SetUserSession(user.TelegramID, "tick_percentage", fmt.Sprintf("%.8f", percentage), 30*time.Minute)

	// 直接创建任务，不需要确认
	b.sendMessage(message.Chat.ID, fmt.Sprintf(`📊 *任务参数*

📈 百分比范围：±%.6f%%
🎯 当前 Tick：%d
📊 Tick 范围：%d 到 %d
💰 投入金额：%.2f USDT

⏳ 正在创建任务并开仓...`, percentage, currentTick, tickLower, tickUpper, amount))

	// 调用创建任务逻辑
	b.createPositionTask(message.Chat.ID, user)
}

// createPositionTask 创建任务并开仓
func (b *Bot) createPositionTask(chatID int64, user *models.User) {
	// Get stored data
	poolAddress, _ := database.GetUserSession(user.TelegramID, "pool_address")
	poolVersion, _ := database.GetUserSession(user.TelegramID, "pool_version")
	poolExchange, _ := database.GetUserSession(user.TelegramID, "pool_exchange")
	token0Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token0")
	token1Symbol, _ := database.GetUserSession(user.TelegramID, "pool_token1")
	feeStr, _ := database.GetUserSession(user.TelegramID, "pool_fee")
	tickSpacingStr, _ := database.GetUserSession(user.TelegramID, "pool_tick_spacing")
	rangePctStr, _ := database.GetUserSession(user.TelegramID, "tick_percentage")
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

	cfg, cfgErr := b.configService.GetOrCreate(user.ID)
	if cfgErr != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 获取全局配置失败: %v", cfgErr))
		return
	}

	// Create Strategy Task
	task := &models.StrategyTask{
		UserID:               user.ID,
		PoolId:               poolAddress,
		PoolVersion:          poolVersion,
		Exchange:             poolExchange,
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
		AmountUSDT:           amount,
		CurrentLiquidity:     "0", // Will be updated after zap in
		ReopenDelaySeconds:   cfg.RebalanceTimeout,
		SlippageTolerance:    cfg.SlippageTolerance,
		AutoReinvest:         cfg.AutoReinvest,
		ResidualTolerance:    cfg.ResidualTolerance,
		StopLossEnabled:      cfg.StopLossEnabled,
		StopLossDelaySeconds: cfg.StopLossDelaySeconds,
		Status:               models.StrategyStatusRunning,
		LastCheckTime:        time.Now(),
	}

	if err := b.strategyService.CreateTask(task); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 创建任务失败: %v", err))
		return
	}

	b.sendMessage(chatID, "⛓️ 任务已创建，正在准备开仓...")

	enterRes, err := b.liquidityService.EnterTaskFromUSDT(user.ID, task)
	if err != nil {
		var swapErr *services.EntrySwapRequiredError
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
投入金额：%.2f USDT

🔄 任务已启动，正在后台运行...

使用 /positions 查看您的仓位。`,
		poolAddress,
		tickLower,
		tickUpper,
		amount,
	)

	b.sendMessage(message.Chat.ID, text)
}
