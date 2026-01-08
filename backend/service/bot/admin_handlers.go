package bot

import (
	"TgLpBot/base/database"
	"TgLpBot/base/models"
	userSvc "TgLpBot/service/user"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleAdmin 处理 /admin 命令 - 显示管理员菜单
func (b *Bot) handleAdmin(message *tgbotapi.Message, user *models.User) {
	// 检查是否是管理员
	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔑 授权码管理", "admin_auth_codes"),
			tgbotapi.NewInlineKeyboardButtonData("👥 用户管理", "admin_users"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📢 发布公告", "admin_announcement"),
		),
	)

	text := `🔐 *管理员控制面板*

您好，管理员！请选择操作：

• *授权码管理* - 生成和管理授权码
• *用户管理* - 查看和管理用户权限
• *发布公告* - 向所有用户发送公告`

	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleAdminAuthCodes 显示授权码管理界面
func (b *Bot) handleAdminAuthCodes(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	// 获取最近的授权码
	var codes []models.AuthCode
	database.DB.Order("created_at DESC").Limit(10).Find(&codes)

	text := "🔑 *授权码管理*\n\n"

	if len(codes) == 0 {
		text += "_暂无授权码_\n"
	} else {
		text += "*最近的授权码:*\n"
		for i, code := range codes {
			status := "✅"
			if code.DisabledAt != nil {
				status = "❌"
			} else if code.ActiveTo != nil && time.Now().After(*code.ActiveTo) {
				status = "⏰"
			} else if code.MaxRedemptions > 0 && code.RedeemedCount >= code.MaxRedemptions {
				status = "🔒"
			}

			activeInfo := "永久"
			if code.ActiveTo != nil {
				activeInfo = fmt.Sprintf("到 %s", code.ActiveTo.Format("01-02"))
			}

			text += fmt.Sprintf("%d. %s `%s` (%d/%d) %s\n",
				i+1, status, code.Code, code.RedeemedCount, code.MaxRedemptions, activeInfo)
		}
		text += "\n_点击下方按钮编辑授权码_"
	}

	// 创建授权码编辑按钮
	var rows [][]tgbotapi.InlineKeyboardButton
	for i, code := range codes {
		if i >= 5 {
			break // 只显示前5个授权码的按钮
		}
		status := "✅"
		if code.DisabledAt != nil {
			status = "❌"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("%s %s...", status, code.Code[:8]),
				fmt.Sprintf("admin_code_%d", code.ID),
			),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ 生成授权码", "admin_create_code"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔙 返回管理菜单", "admin_back"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleAdminCreateCode 显示创建授权码表单
func (b *Bot) handleAdminCreateCode(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	// 快速创建按钮
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 30天/无Auto", "admin_quick_code_30_1_0"),
			tgbotapi.NewInlineKeyboardButtonData("📅 30天/有Auto", "admin_quick_code_30_1_1"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 90天/无Auto", "admin_quick_code_90_1_0"),
			tgbotapi.NewInlineKeyboardButtonData("📅 90天/有Auto", "admin_quick_code_90_1_1"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 永久/无Auto", "admin_quick_code_0_1_0"),
			tgbotapi.NewInlineKeyboardButtonData("📅 永久/有Auto", "admin_quick_code_0_1_1"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ 自定义", "admin_custom_code"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回", "admin_auth_codes"),
		),
	)

	text := `➕ *生成授权码*

选择预设方案或自定义参数：

*预设方案说明:*
• 无Auto - 仅手动开仓，无自动托管
• 有Auto - 可使用自动托管(Auto模式)
• 默认额度：3钱包/3任务

💡 自定义可设置更多参数`

	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleAdminQuickCode 快速生成授权码
func (b *Bot) handleAdminQuickCode(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "正在生成...")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	// 解析参数: admin_quick_code_{days}_{maxRedemptions}_{autoEnabled}
	parts := strings.Split(query.Data, "_")
	if len(parts) < 6 {
		b.sendMessage(query.Message.Chat.ID, "❌ 参数错误")
		return
	}

	days, _ := strconv.Atoi(parts[3])
	maxRedemptions, _ := strconv.Atoi(parts[4])
	autoEnabled := parts[5] == "1"

	var activeTo *time.Time
	if days > 0 {
		t := time.Now().AddDate(0, 0, days)
		activeTo = &t
	}

	input := userSvc.CreateAuthCodeInput{
		ActiveFrom:      nil,
		ActiveTo:        activeTo,
		MaxWallets:      3,
		MaxActiveTasks:  3,
		MaxRedemptions:  maxRedemptions,
		AutoModeEnabled: autoEnabled,
		Note:            fmt.Sprintf("快速生成 %d天/Auto=%v", days, autoEnabled),
	}

	code, err := b.accessService.CreateAuthCode(user.ID, input)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 生成失败: %v", err))
		return
	}

	validityText := "永久有效"
	if days > 0 {
		validityText = fmt.Sprintf("%d 天", days)
	}

	autoText := "❌ 无"
	if autoEnabled {
		autoText = "✅ 有"
	}

	text := fmt.Sprintf(`✅ *授权码已生成*

🔑 授权码: `+"`%s`"+`

📋 *参数:*
• 有效期: %s
• 可使用人数: %d
• 钱包上限: %d
• 任务上限: %d
• Auto模式: %s

复制授权码发送给用户即可。`, code.Code, validityText, maxRedemptions, code.MaxWallets, code.MaxActiveTasks, autoText)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ 继续生成", "admin_create_code"),
			tgbotapi.NewInlineKeyboardButtonData("📋 授权码列表", "admin_auth_codes"),
		),
	)

	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleAdminCustomCode 自定义授权码参数
func (b *Bot) handleAdminCustomCode(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	database.SetUserSession(user.TelegramID, "state", "awaiting_auth_code_params", 10*time.Minute)

	text := `✏️ *自定义授权码参数*

请按以下格式输入参数（用空格分隔）:
` + "`有效天数 使用人数 钱包上限 任务上限 [auto]`" + `

示例:
• ` + "`30 1 3 3`" + ` - 30天/1人/3钱包/3任务/无Auto
• ` + "`90 1 5 5 auto`" + ` - 90天/1人/5钱包/5任务/有Auto
• ` + "`0 1 3 3 auto`" + ` - 永久/1人/3钱包/3任务/有Auto

💡 最后加 auto 表示开通Auto模式权限

输入 /cancel 取消。`

	b.sendMessage(query.Message.Chat.ID, text)
}

// handleAuthCodeParamsInput 处理自定义授权码参数输入
func (b *Bot) handleAuthCodeParamsInput(message *tgbotapi.Message, user *models.User) {
	if !b.accessService.IsAdminUser(user.ID) {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	parts := strings.Fields(message.Text)
	if len(parts) < 4 {
		b.sendMessage(message.Chat.ID, "❌ 参数格式错误，请输入: `有效天数 使用人数 钱包上限 任务上限 [auto]`")
		return
	}

	days, err := strconv.Atoi(parts[0])
	if err != nil || days < 0 {
		b.sendMessage(message.Chat.ID, "❌ 有效天数必须是非负整数")
		return
	}

	maxRedemptions, err := strconv.Atoi(parts[1])
	if err != nil || maxRedemptions < 1 {
		b.sendMessage(message.Chat.ID, "❌ 使用人数必须是正整数")
		return
	}

	maxWallets, err := strconv.Atoi(parts[2])
	if err != nil || maxWallets < 1 {
		b.sendMessage(message.Chat.ID, "❌ 钱包上限必须是正整数")
		return
	}

	maxTasks, err := strconv.Atoi(parts[3])
	if err != nil || maxTasks < 1 {
		b.sendMessage(message.Chat.ID, "❌ 任务上限必须是正整数")
		return
	}

	// 检查是否有 auto 参数
	autoEnabled := false
	if len(parts) >= 5 && strings.ToLower(parts[4]) == "auto" {
		autoEnabled = true
	}

	database.ClearUserSession(user.TelegramID)

	var activeTo *time.Time
	if days > 0 {
		t := time.Now().AddDate(0, 0, days)
		activeTo = &t
	}

	input := userSvc.CreateAuthCodeInput{
		ActiveTo:        activeTo,
		MaxWallets:      maxWallets,
		MaxActiveTasks:  maxTasks,
		MaxRedemptions:  maxRedemptions,
		AutoModeEnabled: autoEnabled,
		Note:            fmt.Sprintf("自定义 %d天/%d人/Auto=%v", days, maxRedemptions, autoEnabled),
	}

	code, err := b.accessService.CreateAuthCode(user.ID, input)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 生成失败: %v", err))
		return
	}

	validityText := "永久有效"
	if days > 0 {
		validityText = fmt.Sprintf("%d 天", days)
	}

	autoText := "❌ 无"
	if autoEnabled {
		autoText = "✅ 有"
	}

	text := fmt.Sprintf(`✅ *授权码已生成*

🔑 授权码: `+"`%s`"+`

📋 *参数:*
• 有效期: %s
• 可使用人数: %d
• 钱包上限: %d
• 任务上限: %d
• Auto模式: %s`, code.Code, validityText, maxRedemptions, maxWallets, maxTasks, autoText)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ 继续生成", "admin_create_code"),
			tgbotapi.NewInlineKeyboardButtonData("📋 授权码列表", "admin_auth_codes"),
		),
	)

	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleAdminUsers 显示用户管理界面（首页）
func (b *Bot) handleAdminUsers(query *tgbotapi.CallbackQuery, user *models.User) {
	b.handleAdminUsersPage(query, user, 1)
}

// handleAdminUsersPage 显示用户管理界面（指定页）
func (b *Bot) handleAdminUsersPage(query *tgbotapi.CallbackQuery, user *models.User, page int) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	pageSize := 8
	accesses, total, err := b.accessService.ListUserAccessesPaged(page, pageSize)
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取用户列表失败: %v", err))
		return
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if totalPages < 1 {
		totalPages = 1
	}

	// 获取统计信息
	activeCount, _ := b.accessService.CountActiveUsers()

	text := fmt.Sprintf("👥 *用户管理*\n\n📊 总用户: %d | 活跃: %d\n📄 第 %d/%d 页\n\n", total, activeCount, page, totalPages)

	if len(accesses) == 0 {
		text += "_暂无授权用户_\n"
	} else {
		for i, access := range accesses {
			status := "✅"
			if access.RevokedAt != nil {
				status = "🚫"
			} else if access.ActiveTo != nil && time.Now().After(*access.ActiveTo) {
				status = "⏰"
			}

			username := fmt.Sprintf("ID:%d", access.User.TelegramID)
			if access.User.Username != "" {
				username = "@" + access.User.Username
			} else if access.User.FirstName != "" {
				username = access.User.FirstName
			}

			text += fmt.Sprintf("%d. %s %s (💼%d 📋%d)\n",
				(page-1)*pageSize+i+1, status, username, access.MaxWallets, access.MaxActiveTasks)
		}
	}

	// 创建按钮
	var rows [][]tgbotapi.InlineKeyboardButton

	// 用户按钮（每行2个）
	for i := 0; i < len(accesses); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{}
		for j := 0; j < 2 && i+j < len(accesses); j++ {
			access := accesses[i+j]
			status := "✅"
			if access.RevokedAt != nil {
				status = "🚫"
			}
			label := fmt.Sprintf("%s", status)
			if access.User.Username != "" {
				label += "@" + access.User.Username
			} else {
				label += fmt.Sprintf("ID:%d", access.User.TelegramID)
			}
			if len(label) > 15 {
				label = label[:15] + ".."
			}
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(
				label,
				fmt.Sprintf("admin_user_%d", access.UserID),
			))
		}
		rows = append(rows, row)
	}

	// 分页按钮
	if totalPages > 1 {
		pageRow := []tgbotapi.InlineKeyboardButton{}
		if page > 1 {
			pageRow = append(pageRow, tgbotapi.NewInlineKeyboardButtonData("⬅️ 上一页", fmt.Sprintf("admin_users_page_%d", page-1)))
		}
		if page < totalPages {
			pageRow = append(pageRow, tgbotapi.NewInlineKeyboardButtonData("➡️ 下一页", fmt.Sprintf("admin_users_page_%d", page+1)))
		}
		rows = append(rows, pageRow)
	}

	// 搜索和返回按钮
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔍 搜索用户", "admin_user_search"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔙 返回管理菜单", "admin_back"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleAdminUserSearch 用户搜索
func (b *Bot) handleAdminUserSearch(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	database.SetUserSession(user.TelegramID, "state", "awaiting_user_search", 10*time.Minute)

	text := `🔍 *搜索用户*

请输入用户名或 Telegram ID：

示例：
• ` + "`@username`" + ` - 按用户名搜索
• ` + "`12345678`" + ` - 按 Telegram ID 搜索

输入 /cancel 取消。`

	b.sendMessage(query.Message.Chat.ID, text)
}

// handleUserSearchInput 处理用户搜索输入
func (b *Bot) handleUserSearchInput(message *tgbotapi.Message, user *models.User) {
	if !b.accessService.IsAdminUser(user.ID) {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	database.ClearUserSession(user.TelegramID)

	query := strings.TrimSpace(message.Text)
	if query == "" {
		b.sendMessage(message.Chat.ID, "❌ 请输入搜索条件")
		return
	}

	accesses, err := b.accessService.SearchUserAccess(query)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 搜索失败: %v", err))
		return
	}

	if len(accesses) == 0 {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔍 重新搜索", "admin_user_search"),
				tgbotapi.NewInlineKeyboardButtonData("👥 用户列表", "admin_users"),
			),
		)
		b.sendMessageWithKeyboard(message.Chat.ID, "❌ 未找到匹配的用户", keyboard)
		return
	}

	text := fmt.Sprintf("🔍 *搜索结果* (共 %d 个)\n\n", len(accesses))
	var rows [][]tgbotapi.InlineKeyboardButton

	for i, access := range accesses {
		status := "✅"
		if access.RevokedAt != nil {
			status = "🚫"
		}
		username := fmt.Sprintf("ID:%d", access.User.TelegramID)
		if access.User.Username != "" {
			username = "@" + access.User.Username
		}
		text += fmt.Sprintf("%d. %s %s (💼%d 📋%d)\n", i+1, status, username, access.MaxWallets, access.MaxActiveTasks)

		if i < 5 {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("📝 %s", username), fmt.Sprintf("admin_user_%d", access.UserID)),
			))
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔍 重新搜索", "admin_user_search"),
		tgbotapi.NewInlineKeyboardButtonData("👥 用户列表", "admin_users"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleAdminUserDetail 显示用户详情
func (b *Bot) handleAdminUserDetail(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	// 解析用户ID
	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		b.sendMessage(query.Message.Chat.ID, "❌ 参数错误")
		return
	}

	targetUserID, _ := strconv.ParseUint(parts[2], 10, 32)

	access, err := b.accessService.GetUserAccessWithUser(uint(targetUserID))
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取用户信息失败: %v", err))
		return
	}

	status := "✅ 已授权"
	if access.RevokedAt != nil {
		status = "🚫 已停用"
	} else if access.ActiveTo != nil && time.Now().After(*access.ActiveTo) {
		status = "⏰ 已过期"
	}

	username := fmt.Sprintf("User#%d", access.UserID)
	if access.User.Username != "" {
		username = "@" + access.User.Username
	}

	activeToText := "永久"
	if access.ActiveTo != nil {
		activeToText = access.ActiveTo.Format("2006-01-02")
	}

	autoModeText := "❌ 无"
	if access.AutoModeEnabled {
		autoModeText = "✅ 有"
	}

	walletCount, _ := b.accessService.CountUserWallets(uint(targetUserID))
	taskCount, _ := b.accessService.CountUserActiveTasks(uint(targetUserID))

	text := fmt.Sprintf(`📝 *用户详情*

👤 用户: %s
🆔 ID: %d
📊 状态: %s

📅 授权到期: %s
💼 钱包: %d / %d
📋 活跃任务: %d / %d
🤖 Auto模式: %s

备注: %s`,
		username, access.User.TelegramID, status,
		activeToText, walletCount, access.MaxWallets,
		taskCount, access.MaxActiveTasks,
		autoModeText,
		access.Note)

	var actionBtn tgbotapi.InlineKeyboardButton
	if access.RevokedAt != nil {
		actionBtn = tgbotapi.NewInlineKeyboardButtonData("✅ 恢复授权", fmt.Sprintf("admin_restore_%d", targetUserID))
	} else {
		actionBtn = tgbotapi.NewInlineKeyboardButtonData("🚫 停用用户", fmt.Sprintf("admin_revoke_%d", targetUserID))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ 编辑权限", fmt.Sprintf("admin_user_edit_%d", targetUserID)),
			actionBtn,
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回用户列表", "admin_users"),
		),
	)

	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleAdminUserEdit 显示编辑用户权限表单
func (b *Bot) handleAdminUserEdit(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	parts := strings.Split(query.Data, "_")
	if len(parts) < 4 {
		b.sendMessage(query.Message.Chat.ID, "❌ 参数错误")
		return
	}

	targetUserID, _ := strconv.ParseUint(parts[3], 10, 32)

	access, err := b.accessService.GetUserAccessWithUser(uint(targetUserID))
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取用户信息失败: %v", err))
		return
	}

	username := fmt.Sprintf("ID:%d", access.User.TelegramID)
	if access.User.Username != "" {
		username = "@" + access.User.Username
	}

	currentExpiry := "永久"
	if access.ActiveTo != nil {
		currentExpiry = access.ActiveTo.Format("2006-01-02")
	}

	currentAuto := "无"
	if access.AutoModeEnabled {
		currentAuto = "有"
	}

	// 保存编辑的用户ID到session
	database.SetUserSession(user.TelegramID, "edit_user_id", fmt.Sprintf("%d", targetUserID), 10*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_user_edit_params", 10*time.Minute)

	text := fmt.Sprintf(`✏️ *编辑用户权限*

👤 用户: %s
当前配置: 钱包=%d, 任务=%d, 到期=%s, Auto=%s

请输入新的配置参数（用空格分隔）:
`+"`钱包 任务 [到期天数] [auto]`"+`

示例:
• `+"`5 5`"+` - 仅修改额度
• `+"`5 5 90`"+` - 额度+90天到期
• `+"`5 5 90 auto`"+` - 额度+90天+开通Auto
• `+"`5 5 0 auto`"+` - 额度+永久+开通Auto

💡 到期天数: 0=永久, 正数=从今天起N天
💡 最后加 auto 表示开通Auto模式

输入 /cancel 取消。`, username, access.MaxWallets, access.MaxActiveTasks, currentExpiry, currentAuto)

	b.sendMessage(query.Message.Chat.ID, text)
}

// handleUserEditInput 处理用户权限编辑输入
func (b *Bot) handleUserEditInput(message *tgbotapi.Message, user *models.User) {
	if !b.accessService.IsAdminUser(user.ID) {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	userIDStr, _ := database.GetUserSession(user.TelegramID, "edit_user_id")
	database.ClearUserSession(user.TelegramID)

	if userIDStr == "" {
		b.sendMessage(message.Chat.ID, "❌ 会话已过期，请重新操作。")
		return
	}

	targetUserID, _ := strconv.ParseUint(userIDStr, 10, 32)

	parts := strings.Fields(message.Text)
	if len(parts) < 2 {
		b.sendMessage(message.Chat.ID, "❌ 参数格式错误，请输入: `钱包 任务 [到期天数] [auto]`")
		return
	}

	maxWallets, err := strconv.Atoi(parts[0])
	if err != nil || maxWallets < 1 {
		b.sendMessage(message.Chat.ID, "❌ 钱包上限必须是正整数")
		return
	}

	maxTasks, err := strconv.Atoi(parts[1])
	if err != nil || maxTasks < 1 {
		b.sendMessage(message.Chat.ID, "❌ 任务上限必须是正整数")
		return
	}

	input := userSvc.UpdateUserAccessInput{
		MaxWallets:     &maxWallets,
		MaxActiveTasks: &maxTasks,
	}

	// 解析可选的到期天数
	if len(parts) >= 3 {
		days, err := strconv.Atoi(parts[2])
		if err == nil && days >= 0 {
			if days == 0 {
				// 永久授权
				input.ActiveTo = nil
			} else {
				t := time.Now().AddDate(0, 0, days)
				input.ActiveTo = &t
			}
		}
	}

	// 解析可选的 auto 参数
	for i := 2; i < len(parts); i++ {
		if strings.ToLower(parts[i]) == "auto" {
			autoEnabled := true
			input.AutoModeEnabled = &autoEnabled
			break
		}
	}

	access, err := b.accessService.UpdateUserAccess(user.ID, uint(targetUserID), input)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 更新失败: %v", err))
		return
	}

	username := fmt.Sprintf("ID:%d", access.User.TelegramID)
	if access.User.Username != "" {
		username = "@" + access.User.Username
	}

	activeToText := "永久"
	if access.ActiveTo != nil {
		activeToText = access.ActiveTo.Format("2006-01-02")
	}

	autoText := "❌ 无"
	if access.AutoModeEnabled {
		autoText = "✅ 有"
	}

	text := fmt.Sprintf(`✅ *用户权限已更新*

👤 用户: %s

📋 *新配置:*
• 钱包上限: %d
• 任务上限: %d
• 授权到期: %s
• Auto模式: %s`, username, access.MaxWallets, access.MaxActiveTasks, activeToText, autoText)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📝 查看详情", fmt.Sprintf("admin_user_%d", targetUserID)),
			tgbotapi.NewInlineKeyboardButtonData("👥 用户列表", "admin_users"),
		),
	)

	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleAdminUserRevoke 停用用户
func (b *Bot) handleAdminUserRevoke(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "正在停用...")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		return
	}

	targetUserID, _ := strconv.ParseUint(parts[2], 10, 32)

	if err := b.accessService.RevokeUserAccess(user.ID, uint(targetUserID)); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 停用失败: %v", err))
		return
	}

	b.sendMessage(query.Message.Chat.ID, "✅ 用户已停用")

	// 刷新用户详情
	query.Data = fmt.Sprintf("admin_user_%d", targetUserID)
	b.handleAdminUserDetail(query, user)
}

// handleAdminUserRestore 恢复用户
func (b *Bot) handleAdminUserRestore(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "正在恢复...")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		return
	}

	targetUserID, _ := strconv.ParseUint(parts[2], 10, 32)

	if err := b.accessService.RestoreUserAccess(user.ID, uint(targetUserID)); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 恢复失败: %v", err))
		return
	}

	b.sendMessage(query.Message.Chat.ID, "✅ 用户已恢复")

	// 刷新用户详情
	query.Data = fmt.Sprintf("admin_user_%d", targetUserID)
	b.handleAdminUserDetail(query, user)
}

// handleAdminAnnouncement 显示发布公告界面
func (b *Bot) handleAdminAnnouncement(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📢 普通公告", "admin_announce_normal"),
			tgbotapi.NewInlineKeyboardButtonData("📌 置顶公告", "admin_announce_pinned"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回管理菜单", "admin_back"),
		),
	)

	text := `📢 *发布公告*

选择公告类型：

• *普通公告* - 发送消息给所有用户
• *置顶公告* - 发送并置顶到用户聊天窗口顶部

置顶公告会在每个用户的聊天窗口中置顶显示，直到被新消息覆盖。`

	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleAdminAnnounceNormal 发布普通公告
func (b *Bot) handleAdminAnnounceNormal(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	database.SetUserSession(user.TelegramID, "state", "awaiting_announcement", 10*time.Minute)
	database.SetUserSession(user.TelegramID, "announce_pinned", "false", 10*time.Minute)

	text := `📢 *发布普通公告*

请输入公告内容，将发送给所有用户。

支持 Markdown 格式。

输入 /cancel 取消。`

	b.sendMessage(query.Message.Chat.ID, text)
}

// handleAdminAnnouncePinned 发布置顶公告
func (b *Bot) handleAdminAnnouncePinned(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	database.SetUserSession(user.TelegramID, "state", "awaiting_announcement", 10*time.Minute)
	database.SetUserSession(user.TelegramID, "announce_pinned", "true", 10*time.Minute)

	text := `📌 *发布置顶公告*

请输入公告内容，将发送并置顶给所有用户。

支持 Markdown 格式。

⚠️ 置顶消息会覆盖用户之前的置顶消息。

输入 /cancel 取消。`

	b.sendMessage(query.Message.Chat.ID, text)
}

// handleAnnouncementInput 处理公告内容输入
func (b *Bot) handleAnnouncementInput(message *tgbotapi.Message, user *models.User) {
	if !b.accessService.IsAdminUser(user.ID) {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	isPinned, _ := database.GetUserSession(user.TelegramID, "announce_pinned")
	database.ClearUserSession(user.TelegramID)

	content := strings.TrimSpace(message.Text)
	if content == "" {
		b.sendMessage(message.Chat.ID, "❌ 公告内容不能为空")
		return
	}

	// 保存公告到数据库
	announcement := models.Announcement{
		CreatedByUserID: user.ID,
		Title:           "系统公告",
		Content:         content,
	}
	database.DB.Create(&announcement)

	// 获取所有用户
	var users []models.User
	database.DB.Find(&users)

	successCount := 0
	failCount := 0
	pinnedCount := 0

	prefix := "📢"
	if isPinned == "true" {
		prefix = "📌"
	}
	announcementText := fmt.Sprintf("%s *系统公告*\n\n%s\n\n_— 管理员 %s_", prefix, content, time.Now().Format("01-02 15:04"))

	for _, u := range users {
		if u.ID == user.ID {
			continue // 跳过发送给自己
		}
		sentMsg, err := b.sendMessage(u.TelegramID, announcementText)
		if err != nil {
			failCount++
		} else {
			successCount++
			// 如果需要置顶
			if isPinned == "true" && sentMsg.MessageID != 0 {
				pinConfig := tgbotapi.PinChatMessageConfig{
					ChatID:              u.TelegramID,
					MessageID:           sentMsg.MessageID,
					DisableNotification: false,
				}
				if _, err := b.api.Request(pinConfig); err == nil {
					pinnedCount++
				}
			}
		}
	}

	resultText := fmt.Sprintf("✅ 公告已发布\n\n成功发送: %d\n失败: %d", successCount, failCount)
	if isPinned == "true" {
		resultText += fmt.Sprintf("\n置顶成功: %d", pinnedCount)
	}

	b.sendMessage(message.Chat.ID, resultText)
}

// handleAdminBack 返回管理员主菜单
func (b *Bot) handleAdminBack(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔑 授权码管理", "admin_auth_codes"),
			tgbotapi.NewInlineKeyboardButtonData("👥 用户管理", "admin_users"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📢 发布公告", "admin_announcement"),
		),
	)

	text := `🔐 *管理员控制面板*

请选择操作：`

	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleRedeemCode 处理用户输入的授权码
func (b *Bot) handleRedeemCode(message *tgbotapi.Message, user *models.User) {
	database.ClearUserSession(user.TelegramID)

	code := strings.TrimSpace(message.Text)
	if code == "" {
		b.sendMessage(message.Chat.ID, "❌ 授权码不能为空，请重新输入。")
		return
	}

	access, authCode, err := b.accessService.RedeemAuthCode(user.ID, code)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 授权码无效: %v\n\n请检查授权码是否正确，或联系管理员获取新的授权码。", err))
		return
	}

	activeToText := "永久有效"
	if access.ActiveTo != nil {
		activeToText = fmt.Sprintf("到 %s", access.ActiveTo.Format("2006-01-02"))
	}

	text := fmt.Sprintf(`✅ *授权成功！*

恭喜您已成功激活 Bot！

📋 *您的权限:*
• 有效期: %s
• 钱包上限: %d
• 任务上限: %d

现在您可以使用 /wallet 导入钱包开始使用了。`, activeToText, authCode.MaxWallets, authCode.MaxActiveTasks)

	b.sendMessage(message.Chat.ID, text)
}

// checkUserAuthorized 检查用户是否已授权
func (b *Bot) checkUserAuthorized(chatID int64, user *models.User) bool {
	check, _ := b.accessService.CheckUserAccess(user.ID, time.Now())
	if check.Allowed {
		return true
	}

	// 用户未授权，提示输入授权码
	database.SetUserSession(user.TelegramID, "state", "awaiting_auth_code", 30*time.Minute)

	text := `⚠️ *需要授权*

您尚未获得使用授权。

请输入授权码进行激活，或联系管理员获取授权码。`

	b.sendMessage(chatID, text)
	return false
}

// formatAccessStatus 格式化用户授权状态
func (b *Bot) formatAccessStatus(user *models.User) string {
	check, _ := b.accessService.CheckUserAccess(user.ID, time.Now())

	if check.IsAdmin {
		return "\n\n🔐 *管理员权限*\n使用 /admin 进入管理员菜单"
	}

	if !check.Allowed {
		return "\n\n⚠️ *未授权* - 请输入授权码激活 Bot"
	}

	if check.Access != nil && check.Access.ActiveTo != nil {
		return fmt.Sprintf("\n\n✅ *已授权* (到期: %s)", check.Access.ActiveTo.Format("2006-01-02"))
	}

	return "\n\n✅ *已授权* (永久有效)"
}

// handleAdminCodeDetail 显示授权码详情
func (b *Bot) handleAdminCodeDetail(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	// 解析授权码ID
	parts := strings.Split(query.Data, "_")
	if len(parts) < 3 {
		b.sendMessage(query.Message.Chat.ID, "❌ 参数错误")
		return
	}

	codeID, _ := strconv.ParseUint(parts[2], 10, 32)

	code, err := b.accessService.GetAuthCode(uint(codeID))
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取授权码失败: %v", err))
		return
	}

	status := "✅ 有效"
	if code.DisabledAt != nil {
		status = "❌ 已停用"
	} else if code.ActiveTo != nil && time.Now().After(*code.ActiveTo) {
		status = "⏰ 已过期"
	} else if code.MaxRedemptions > 0 && code.RedeemedCount >= code.MaxRedemptions {
		status = "🔒 已用完"
	}

	activeToText := "永久"
	if code.ActiveTo != nil {
		activeToText = code.ActiveTo.Format("2006-01-02")
	}

	text := fmt.Sprintf(`🔑 *授权码详情*

📝 授权码: `+"`%s`"+`
📊 状态: %s

📅 有效期至: %s
👥 使用次数: %d / %d
💼 钱包上限: %d
📋 任务上限: %d

📝 备注: %s`,
		code.Code, status, activeToText,
		code.RedeemedCount, code.MaxRedemptions,
		code.MaxWallets, code.MaxActiveTasks,
		code.Note)

	var actionBtn tgbotapi.InlineKeyboardButton
	if code.DisabledAt != nil {
		actionBtn = tgbotapi.NewInlineKeyboardButtonData("✅ 启用", fmt.Sprintf("admin_code_enable_%d", codeID))
	} else {
		actionBtn = tgbotapi.NewInlineKeyboardButtonData("❌ 停用", fmt.Sprintf("admin_code_disable_%d", codeID))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ 编辑额度", fmt.Sprintf("admin_code_edit_%d", codeID)),
			actionBtn,
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回授权码列表", "admin_auth_codes"),
		),
	)

	b.sendMessageWithKeyboard(query.Message.Chat.ID, text, keyboard)
}

// handleAdminCodeEdit 显示编辑授权码表单
func (b *Bot) handleAdminCodeEdit(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	// 解析授权码ID
	parts := strings.Split(query.Data, "_")
	if len(parts) < 4 {
		b.sendMessage(query.Message.Chat.ID, "❌ 参数错误")
		return
	}

	codeID, _ := strconv.ParseUint(parts[3], 10, 32)

	code, err := b.accessService.GetAuthCode(uint(codeID))
	if err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 获取授权码失败: %v", err))
		return
	}

	// 保存编辑的授权码ID到session
	database.SetUserSession(user.TelegramID, "edit_code_id", fmt.Sprintf("%d", codeID), 10*time.Minute)
	database.SetUserSession(user.TelegramID, "state", "awaiting_code_edit_params", 10*time.Minute)

	text := fmt.Sprintf(`✏️ *编辑授权码*

当前授权码: `+"`%s`"+`
当前参数: 使用人数=%d, 钱包=%d, 任务=%d

请输入新的参数（用空格分隔）:
`+"`使用人数 钱包上限 任务上限`"+`

示例: `+"`5 3 3`"+` - 最多5人使用，每人3钱包、3任务

输入 /cancel 取消。`, code.Code, code.MaxRedemptions, code.MaxWallets, code.MaxActiveTasks)

	b.sendMessage(query.Message.Chat.ID, text)
}

// handleCodeEditInput 处理授权码编辑输入
func (b *Bot) handleCodeEditInput(message *tgbotapi.Message, user *models.User) {
	if !b.accessService.IsAdminUser(user.ID) {
		database.ClearUserSession(user.TelegramID)
		b.sendMessage(message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	codeIDStr, _ := database.GetUserSession(user.TelegramID, "edit_code_id")
	database.ClearUserSession(user.TelegramID)

	if codeIDStr == "" {
		b.sendMessage(message.Chat.ID, "❌ 会话已过期，请重新操作。")
		return
	}

	codeID, _ := strconv.ParseUint(codeIDStr, 10, 32)

	parts := strings.Fields(message.Text)
	if len(parts) < 3 {
		b.sendMessage(message.Chat.ID, "❌ 参数格式错误，请输入: `使用人数 钱包上限 任务上限`")
		return
	}

	maxRedemptions, err := strconv.Atoi(parts[0])
	if err != nil || maxRedemptions < 1 {
		b.sendMessage(message.Chat.ID, "❌ 使用人数必须是正整数")
		return
	}

	maxWallets, err := strconv.Atoi(parts[1])
	if err != nil || maxWallets < 1 {
		b.sendMessage(message.Chat.ID, "❌ 钱包上限必须是正整数")
		return
	}

	maxTasks, err := strconv.Atoi(parts[2])
	if err != nil || maxTasks < 1 {
		b.sendMessage(message.Chat.ID, "❌ 任务上限必须是正整数")
		return
	}

	input := userSvc.UpdateAuthCodeInput{
		MaxRedemptions: &maxRedemptions,
		MaxWallets:     &maxWallets,
		MaxActiveTasks: &maxTasks,
	}

	code, err := b.accessService.UpdateAuthCode(uint(codeID), input)
	if err != nil {
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ 更新失败: %v", err))
		return
	}

	text := fmt.Sprintf(`✅ *授权码已更新*

🔑 授权码: `+"`%s`"+`

📋 *新参数:*
• 可使用人数: %d
• 钱包上限: %d
• 任务上限: %d`, code.Code, code.MaxRedemptions, code.MaxWallets, code.MaxActiveTasks)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 返回授权码列表", "admin_auth_codes"),
		),
	)

	b.sendMessageWithKeyboard(message.Chat.ID, text, keyboard)
}

// handleAdminCodeDisable 停用授权码
func (b *Bot) handleAdminCodeDisable(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "正在停用...")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	parts := strings.Split(query.Data, "_")
	if len(parts) < 4 {
		return
	}

	codeID, _ := strconv.ParseUint(parts[3], 10, 32)

	if err := b.accessService.DisableAuthCode(uint(codeID)); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 停用失败: %v", err))
		return
	}

	b.sendMessage(query.Message.Chat.ID, "✅ 授权码已停用")

	// 刷新详情
	query.Data = fmt.Sprintf("admin_code_%d", codeID)
	b.handleAdminCodeDetail(query, user)
}

// handleAdminCodeEnable 启用授权码
func (b *Bot) handleAdminCodeEnable(query *tgbotapi.CallbackQuery, user *models.User) {
	callback := tgbotapi.NewCallback(query.ID, "正在启用...")
	b.api.Send(callback)

	if !b.accessService.IsAdminUser(user.ID) {
		b.sendMessage(query.Message.Chat.ID, "⚠️ 您没有管理员权限。")
		return
	}

	parts := strings.Split(query.Data, "_")
	if len(parts) < 4 {
		return
	}

	codeID, _ := strconv.ParseUint(parts[3], 10, 32)

	if err := b.accessService.EnableAuthCode(uint(codeID)); err != nil {
		b.sendMessage(query.Message.Chat.ID, fmt.Sprintf("❌ 启用失败: %v", err))
		return
	}

	b.sendMessage(query.Message.Chat.ID, "✅ 授权码已启用")

	// 刷新详情
	query.Data = fmt.Sprintf("admin_code_%d", codeID)
	b.handleAdminCodeDetail(query, user)
}
