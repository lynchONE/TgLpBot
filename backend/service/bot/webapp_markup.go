package bot

// Note: go-telegram-bot-api/v5.5.1 doesn't include WebApp fields on InlineKeyboardButton.
// We send WebApp inline keyboard markup via custom JSON structures.

import (
	"net/url"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type webAppInfo struct {
	URL string `json:"url"`
}

type webAppInlineKeyboardButton struct {
	Text         string      `json:"text"`
	CallbackData *string     `json:"callback_data,omitempty"`
	WebApp       *webAppInfo `json:"web_app,omitempty"`
}

type webAppInlineKeyboardMarkup struct {
	InlineKeyboard [][]webAppInlineKeyboardButton `json:"inline_keyboard"`
}

func newWebAppInlineKeyboardMarkup(buttonText, url string) webAppInlineKeyboardMarkup {
	return webAppInlineKeyboardMarkup{
		InlineKeyboard: [][]webAppInlineKeyboardButton{
			{
				{
					Text:   buttonText,
					WebApp: &webAppInfo{URL: strings.TrimSpace(url)},
				},
			},
		},
	}
}

func newInlineKeyboardMarkupWithWebAppRow(base tgbotapi.InlineKeyboardMarkup, buttonText, url string) any {
	url = strings.TrimSpace(url)
	if !isValidWebAppURL(url) {
		return base
	}

	out := webAppInlineKeyboardMarkup{
		InlineKeyboard: make([][]webAppInlineKeyboardButton, 0, len(base.InlineKeyboard)+1),
	}
	for i := range base.InlineKeyboard {
		row := base.InlineKeyboard[i]
		outRow := make([]webAppInlineKeyboardButton, 0, len(row))
		for j := range row {
			btn := row[j]
			outRow = append(outRow, webAppInlineKeyboardButton{
				Text:         btn.Text,
				CallbackData: btn.CallbackData,
			})
		}
		out.InlineKeyboard = append(out.InlineKeyboard, outRow)
	}

	out.InlineKeyboard = append(out.InlineKeyboard, []webAppInlineKeyboardButton{
		{
			Text:   buttonText,
			WebApp: &webAppInfo{URL: url},
		},
	})

	return out
}

func isValidWebAppURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return false
	}
	return true
}
