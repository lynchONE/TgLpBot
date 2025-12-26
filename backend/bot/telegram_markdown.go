package bot

import "strings"

var telegramMarkdownReplacer = strings.NewReplacer(
	"\\", "\\\\",
	"_", "\\_",
	"*", "\\*",
	"[", "\\[",
	"]", "\\]",
	"`", "\\`",
)

func escapeTelegramMarkdown(s string) string {
	if s == "" {
		return s
	}
	return telegramMarkdownReplacer.Replace(s)
}
