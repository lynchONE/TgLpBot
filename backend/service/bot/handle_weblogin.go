package bot

import (
	"log"
	"strings"

	"TgLpBot/base/models"
	"TgLpBot/base/webloginstore"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleWebLogin(message *tgbotapi.Message, user *models.User) {
	args := strings.TrimSpace(message.CommandArguments())
	if args == "" {
		b.sendMessage(message.Chat.ID, "请提供验证码。\n\n用法: `/weblogin 验证码`\n\n验证码可在网页登录页面获取。")
		return
	}

	code := strings.ToUpper(strings.Fields(args)[0])
	photoURL := fetchUserPhotoURL(user.TelegramID, b.api)

	ok, errMsg := webloginstore.Confirm(code, user, photoURL)
	if !ok {
		b.sendMessage(message.Chat.ID, "❌ "+errMsg)
		return
	}

	b.sendMessage(message.Chat.ID, "✅ 网页登录验证成功！请返回网页查看。")
}

func fetchUserPhotoURL(telegramID int64, botAPI *tgbotapi.BotAPI) string {
	if botAPI == nil {
		return ""
	}
	photos, err := botAPI.GetUserProfilePhotos(tgbotapi.UserProfilePhotosConfig{
		UserID: telegramID,
		Limit:  1,
	})
	if err != nil || photos.TotalCount == 0 || len(photos.Photos) == 0 || len(photos.Photos[0]) == 0 {
		return ""
	}

	photo := photos.Photos[0][0]
	fileURL, err := botAPI.GetFileDirectURL(photo.FileID)
	if err != nil {
		log.Printf("weblogin: failed to get photo URL for user %d: %v", telegramID, err)
		return ""
	}
	return fileURL
}
