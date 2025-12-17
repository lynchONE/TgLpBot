package bot

// Note: go-telegram-bot-api/v5.5.1 doesn't include WebApp fields on InlineKeyboardButton.
// We send WebApp inline keyboard markup via custom JSON structures.

type webAppInfo struct {
	URL string `json:"url"`
}

type webAppInlineKeyboardButton struct {
	Text   string      `json:"text"`
	WebApp *webAppInfo `json:"web_app,omitempty"`
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
					WebApp: &webAppInfo{URL: url},
				},
			},
		},
	}
}
