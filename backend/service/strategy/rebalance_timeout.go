package strategy

import "fmt"

// NormalizeRebalanceTimeout keeps backward compatibility with legacy 0=immediate
// inputs while storing the new sentinel value -1 for immediate execution.
func NormalizeRebalanceTimeout(seconds int) int {
	if seconds == 0 {
		return -1
	}
	return seconds
}

func FormatDelayTime(seconds int) string {
	if seconds <= 0 {
		return "立即"
	}
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	return fmt.Sprintf("%d 分钟", seconds/60)
}
