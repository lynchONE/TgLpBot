package web_server

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"TgLpBot/base/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (s *Server) handleMeAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	initData := initDataFromQuery(r)
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
	if status, msg := requireMiniAppPermission(check); status != 0 {
		http.Error(w, msg, status)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	body, contentType, err := fetchTelegramUserAvatar(ctx, user.TelegramID)
	if err != nil {
		http.Error(w, "avatar not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(body)
}

func fetchTelegramUserAvatar(ctx context.Context, telegramID int64) ([]byte, string, error) {
	if telegramID == 0 {
		return nil, "", fmt.Errorf("telegram id is empty")
	}
	if config.AppConfig == nil {
		return nil, "", fmt.Errorf("config not loaded")
	}

	botToken := strings.TrimSpace(config.AppConfig.TelegramBotToken)
	if botToken == "" {
		return nil, "", fmt.Errorf("telegram bot token not configured")
	}

	botAPI, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return nil, "", err
	}

	photos, err := botAPI.GetUserProfilePhotos(tgbotapi.UserProfilePhotosConfig{
		UserID: telegramID,
		Limit:  1,
	})
	if err != nil {
		return nil, "", err
	}
	if photos.TotalCount == 0 || len(photos.Photos) == 0 || len(photos.Photos[0]) == 0 {
		return nil, "", fmt.Errorf("telegram avatar not found")
	}

	sizes := photos.Photos[0]
	photo := sizes[len(sizes)-1]
	fileURL, err := botAPI.GetFileDirectURL(photo.FileID)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("telegram avatar download failed: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	if len(body) == 0 {
		return nil, "", fmt.Errorf("telegram avatar is empty")
	}

	return body, detectImageContentType(fileURL, resp.Header.Get("Content-Type"), body), nil
}

func detectImageContentType(fileURL string, headerValue string, body []byte) string {
	contentType := strings.TrimSpace(headerValue)
	if contentType != "" && !strings.EqualFold(contentType, "application/octet-stream") {
		if index := strings.Index(contentType, ";"); index >= 0 {
			contentType = strings.TrimSpace(contentType[:index])
		}
		if contentType != "" {
			return contentType
		}
	}

	if ext := strings.ToLower(path.Ext(fileURL)); ext != "" {
		if guessed := mime.TypeByExtension(ext); guessed != "" {
			return guessed
		}
	}

	return http.DetectContentType(body)
}
