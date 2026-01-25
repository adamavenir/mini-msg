package chat

func (m *Model) calculateThreadPanelWidth() {
	// Calculate width based on longest visible thread name + 4 char buffer, max 50
	// Minimum 26 for good visual balance even with short thread names
	// (ensures "   <space> to filter" (20 chars) fits with comfortable margins)
	maxLen := 26
	entries := m.threadEntries()
	for _, entry := range entries {
		label := m.threadEntryLabel(entry)
		// Strip ANSI codes for accurate length calculation
		cleanLabel := stripANSI(label)
		if len(cleanLabel) > maxLen {
			maxLen = len(cleanLabel)
		}
	}
	// Add 4 char buffer (1 for leading space in render + 3 for padding)
	width := maxLen + 4
	// Max 50 chars
	if width > 50 {
		width = 50
	}
	m.cachedThreadPanelWidth = width
}

func stripANSI(s string) string {
	// Simple ANSI code stripper for length calculation
	result := ""
	inEscape := false
	for _, ch := range s {
		if ch == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEscape = false
			}
			continue
		}
		result += string(ch)
	}
	return result
}

func wrapLine(value string, maxLen int) []string {
	if maxLen <= 0 {
		return []string{value}
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return []string{value}
	}
	// Wrap with 2-char indent on continuation lines
	lines := make([]string, 0)
	firstLineMax := maxLen
	contLineMax := maxLen - 2 // 2-char indent

	if contLineMax < 1 {
		// Width too small to wrap, just truncate
		return []string{string(runes[:maxLen])}
	}

	// First line
	lines = append(lines, string(runes[:firstLineMax]))
	remaining := runes[firstLineMax:]

	// Continuation lines with 2-char indent
	for len(remaining) > 0 {
		if len(remaining) <= contLineMax {
			lines = append(lines, "  "+string(remaining))
			break
		}
		lines = append(lines, "  "+string(remaining[:contLineMax]))
		remaining = remaining[contLineMax:]
	}
	return lines
}
