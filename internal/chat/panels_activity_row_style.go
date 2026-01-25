package chat

import (
	"sort"

	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

type agentRowStyle struct {
	iconColor   lipgloss.Color
	textColor   lipgloss.Color
	statusColor lipgloss.Color
	usedBg      lipgloss.Color
	unusedBg    lipgloss.Color
	fillChars   int
}

func agentRowStyleFor(presence types.PresenceState, sessionMode string, statusDisplay *StatusDisplay, agentColor lipgloss.Color, tokenPercent float64, rowWidth int) agentRowStyle {
	isNewSession := sessionMode == "n"
	var iconColor lipgloss.Color
	switch presence {
	case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted:
		if isNewSession {
			iconColor = lipgloss.Color("117") // light blue - new session spinning up
		} else {
			iconColor = lipgloss.Color("226") // bright yellow - resumed session spinning up
		}
	case types.PresenceCompacting:
		iconColor = lipgloss.Color("226") // bright yellow - compacting context
	case types.PresenceActive:
		iconColor = lipgloss.Color("46") // bright green - active
	case types.PresenceIdle:
		iconColor = lipgloss.Color("250") // dim white - idle
	case types.PresenceOffline:
		iconColor = lipgloss.Color("250") // dim white - offline
	case types.PresenceError:
		iconColor = lipgloss.Color("196") // red - error
	case types.PresenceBRB:
		iconColor = lipgloss.Color("226") // bright yellow - will respawn
	default:
		iconColor = lipgloss.Color("240") // gray fallback
	}

	textColor := agentColor
	statusColor := lipgloss.Color("240")
	usedBg := lipgloss.Color("236")
	unusedBg := lipgloss.Color("233")

	inDangerZone := tokenPercent > agentRowDangerThreshold
	isOffline := presence == types.PresenceOffline
	isError := presence == types.PresenceError

	if isError {
		usedBg = lipgloss.Color("196")
		unusedBg = lipgloss.Color("52")
		textColor = lipgloss.Color("231")
		statusColor = lipgloss.Color("231")
	} else if isOffline {
		usedBg = lipgloss.Color("233")
		unusedBg = lipgloss.Color("233")
		textColor = lipgloss.Color("240")
		statusColor = lipgloss.Color("240")
	} else if inDangerZone {
		usedBg = lipgloss.Color("196")
		unusedBg = lipgloss.Color("233")
		textColor = lipgloss.Color("231")
		statusColor = lipgloss.Color("231")
	}

	if statusDisplay != nil {
		if statusDisplay.IconColor != nil {
			iconColor = lipgloss.Color(*statusDisplay.IconColor)
		}
		if statusDisplay.UsrColor != nil {
			textColor = lipgloss.Color(*statusDisplay.UsrColor)
		}
		if statusDisplay.MsgColor != nil {
			statusColor = lipgloss.Color(*statusDisplay.MsgColor)
		}
		if statusDisplay.UsedTokColor != nil {
			usedBg = lipgloss.Color(*statusDisplay.UsedTokColor)
		}
		if statusDisplay.UnusedTokColor != nil {
			unusedBg = lipgloss.Color(*statusDisplay.UnusedTokColor)
		}
		if statusDisplay.BgColor != nil {
			usedBg = lipgloss.Color(*statusDisplay.BgColor)
			unusedBg = lipgloss.Color(*statusDisplay.BgColor)
		}
	}

	fillChars := agentRowFillChars(tokenPercent, rowWidth, isError, isOffline, inDangerZone)

	return agentRowStyle{
		iconColor:   iconColor,
		textColor:   textColor,
		statusColor: statusColor,
		usedBg:      usedBg,
		unusedBg:    unusedBg,
		fillChars:   fillChars,
	}
}

func agentRowFillChars(tokenPercent float64, rowWidth int, isError bool, isOffline bool, inDangerZone bool) int {
	fillRatio := 0.0
	switch {
	case inDangerZone:
		fillRatio = (tokenPercent - agentRowDangerThreshold) / (1.0 - agentRowDangerThreshold)
		if fillRatio > 1.0 {
			fillRatio = 1.0
		}
	case isError || isOffline:
		if isError {
			fillRatio = 1.0
		} else {
			fillRatio = 0
		}
	default:
		fillRatio = tokenPercent / agentRowTargetPercent
		if fillRatio > 1.0 {
			fillRatio = 1.0
		}
	}

	fillChars := int(fillRatio*float64(rowWidth) + 0.99)
	if fillChars < 0 {
		fillChars = 0
	}
	if fillChars > rowWidth {
		fillChars = rowWidth
	}
	return fillChars
}

func renderAgentRowText(content agentRowContent, style agentRowStyle) string {
	usedIconStyle := lipgloss.NewStyle().Foreground(style.iconColor).Background(style.usedBg).Bold(true)
	usedNameStyle := lipgloss.NewStyle().Foreground(style.textColor).Background(style.usedBg).Bold(true)
	usedStatusStyle := lipgloss.NewStyle().Foreground(style.statusColor).Background(style.usedBg).Italic(true)
	unusedIconStyle := lipgloss.NewStyle().Foreground(style.iconColor).Background(style.unusedBg).Bold(true)
	unusedNameStyle := lipgloss.NewStyle().Foreground(style.textColor).Background(style.unusedBg).Bold(true)
	unusedStatusStyle := lipgloss.NewStyle().Foreground(style.statusColor).Background(style.unusedBg).Italic(true)

	runes := []rune(content.padded)
	var rowText string

	renderRange := func(start, end int) string {
		if start >= end || start >= len(runes) {
			return ""
		}
		if end > len(runes) {
			end = len(runes)
		}
		text := string(runes[start:end])

		inUsed := start < style.fillChars
		inIcon := start < content.iconEndPos
		inName := start >= content.iconEndPos && start < content.boldEndPos

		if inUsed {
			switch {
			case inIcon:
				return usedIconStyle.Render(text)
			case inName:
				return usedNameStyle.Render(text)
			default:
				return usedStatusStyle.Render(text)
			}
		}

		switch {
		case inIcon:
			return unusedIconStyle.Render(text)
		case inName:
			return unusedNameStyle.Render(text)
		default:
			return unusedStatusStyle.Render(text)
		}
	}

	boundaries := []int{0, content.iconEndPos, content.boldEndPos, style.fillChars, content.rowWidth}
	seen := make(map[int]bool)
	unique := make([]int, 0, len(boundaries))
	for _, b := range boundaries {
		if b >= 0 && b <= content.rowWidth && !seen[b] {
			seen[b] = true
			unique = append(unique, b)
		}
	}
	sort.Ints(unique)

	for i := 0; i < len(unique)-1; i++ {
		rowText += renderRange(unique[i], unique[i+1])
	}

	return rowText
}
