package bot

import (
	"fmt"
	"regexp"
	"strconv"

	"TgLpBot/service/strategy"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	rebalanceTimeoutLineRe = regexp.MustCompile(`(\x{518D}\x{5E73}\x{8861}\x{8D85}\x{65F6})([：:]\s*)(-?\d+)\s*(\x{79D2})`)
	rebalanceTimeoutBtnRe  = regexp.MustCompile(`(\x{518D}\x{5E73}\x{8861}\x{8D85}\x{65F6})\s*\((-?\d+)s\)`)
)

func rewriteRebalanceTimeoutText(text string) string {
	text = rebalanceTimeoutLineRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := rebalanceTimeoutLineRe.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		seconds, err := strconv.Atoi(parts[3])
		if err != nil {
			return match
		}
		return fmt.Sprintf("%s%s%s", parts[1], parts[2], strategy.FormatDelayTime(seconds))
	})

	text = rebalanceTimeoutBtnRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := rebalanceTimeoutBtnRe.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		seconds, err := strconv.Atoi(parts[2])
		if err != nil {
			return match
		}
		return fmt.Sprintf("%s (%s)", parts[1], strategy.FormatDelayTime(seconds))
	})

	return text
}

func rewriteReplyMarkupRebalanceTimeout(replyMarkup any) any {
	switch markup := replyMarkup.(type) {
	case tgbotapi.InlineKeyboardMarkup:
		out := markup
		for i := range out.InlineKeyboard {
			for j := range out.InlineKeyboard[i] {
				out.InlineKeyboard[i][j].Text = rewriteRebalanceTimeoutText(out.InlineKeyboard[i][j].Text)
			}
		}
		return out
	case *tgbotapi.InlineKeyboardMarkup:
		if markup == nil {
			return replyMarkup
		}
		out := *markup
		for i := range out.InlineKeyboard {
			for j := range out.InlineKeyboard[i] {
				out.InlineKeyboard[i][j].Text = rewriteRebalanceTimeoutText(out.InlineKeyboard[i][j].Text)
			}
		}
		return &out
	default:
		return replyMarkup
	}
}
