package chat

import "strings"

func truncatePreview(body string, maxLen int) string {
	compact := strings.Join(strings.Fields(body), " ")
	if len(compact) <= maxLen {
		return compact
	}
	return compact[:maxLen-3] + "..."
}

func truncateLine(value string, maxLen int) string {
	if maxLen <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}
