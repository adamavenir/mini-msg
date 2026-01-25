package chat

import (
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/types"
)

const (
	agentRowMaxTokens       = 200000
	agentRowTargetPercent   = 0.80
	agentRowDangerThreshold = 0.80
)

type agentRowContent struct {
	padded     string
	iconEndPos int
	boldEndPos int
	rowWidth   int
}

func (m *Model) agentRowDisplayPresence(agent types.Agent) types.PresenceState {
	displayPresence := agent.Presence
	if dp, ok := m.agentDisplayPresence[agent.AgentID]; ok {
		displayPresence = dp
	}
	return displayPresence
}

func agentPresenceIcon(presence types.PresenceState, animationFrame int) string {
	// Spawn cycle: △ ▲ animate slowly (1.5s cycle = 6 frames at 250ms each, 3 frames per icon)
	spawnCycleIcons := []string{"△", "△", "△", "▲", "▲", "▲"}
	// Compact cycle: ◁ ◀ animate slowly (same timing)
	compactCycleIcons := []string{"◁", "◁", "◁", "◀", "◀", "◀"}

	icon := "▶"
	switch presence {
	case types.PresenceActive:
		icon = "▶"
	case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted:
		icon = spawnCycleIcons[animationFrame%len(spawnCycleIcons)]
	case types.PresenceCompacting:
		icon = compactCycleIcons[animationFrame%len(compactCycleIcons)]
	case types.PresenceIdle:
		icon = "▷"
	case types.PresenceError:
		icon = "𝘅"
	case types.PresenceOffline:
		icon = "▽"
	case types.PresenceBRB:
		icon = "◁" // BRB - will respawn immediately
	}
	return icon
}

func agentRowStatus(agent types.Agent, presence types.PresenceState, animationFrame int, statusInvoker *StatusInvoker) (string, string, *StatusDisplay) {
	status := ""
	if agent.Status != nil && *agent.Status != "" {
		status = *agent.Status
	}

	icon := agentPresenceIcon(presence, animationFrame)
	if presence == types.PresenceIdle && status != "" {
		statusLower := strings.ToLower(status)
		switch {
		case strings.HasPrefix(statusLower, "awaiting:") || strings.HasPrefix(statusLower, "waiting:"):
			icon = "⧗"
		case strings.HasPrefix(statusLower, "done:") || strings.HasPrefix(statusLower, "complete:"):
			icon = "✓"
		case strings.HasPrefix(statusLower, "blocked:") || strings.HasPrefix(statusLower, "stuck:"):
			icon = "⚠"
		}
	}

	var statusDisplay *StatusDisplay
	if statusInvoker != nil && status != "" {
		statusDisplay = statusInvoker.GetDisplay(status)
	}

	displayStatus := status
	if statusDisplay != nil {
		if statusDisplay.Icon != nil {
			icon = *statusDisplay.Icon
		}
		if statusDisplay.Message != nil {
			displayStatus = *statusDisplay.Message
		}
	}

	return icon, displayStatus, statusDisplay
}

func buildAgentRowContent(agent types.Agent, icon, displayStatus string, unread, rowWidth int) agentRowContent {
	if rowWidth < 0 {
		rowWidth = 0
	}

	name := agent.AgentID
	iconPart := fmt.Sprintf(" %s", icon)
	namePart := fmt.Sprintf(" %s", name)

	isForkSession := agent.SessionMode != "" && agent.SessionMode != "n"
	if isForkSession {
		namePart = fmt.Sprintf(" %s#%s", name, agent.SessionMode)
	}

	italicPart := ""
	if displayStatus != "" {
		italicPart += " " + displayStatus
	}
	if unread > 0 {
		italicPart += fmt.Sprintf(" (%d)", unread)
	}

	iconEndPos := len([]rune(iconPart))
	boldEndPos := iconEndPos + len([]rune(namePart))

	content := iconPart + namePart + italicPart
	contentRunes := []rune(content)
	if len(contentRunes) > rowWidth {
		contentRunes = contentRunes[:rowWidth]
		if boldEndPos > rowWidth {
			boldEndPos = rowWidth
		}
	}
	for len(contentRunes) < rowWidth {
		contentRunes = append(contentRunes, ' ')
	}

	return agentRowContent{
		padded:     string(contentRunes),
		iconEndPos: iconEndPos,
		boldEndPos: boldEndPos,
		rowWidth:   rowWidth,
	}
}

func agentRowTokenPercent(usage *TokenUsage) float64 {
	if usage == nil {
		return 0
	}
	contextTokens := usage.ContextTokens()
	if contextTokens <= 0 {
		return 0
	}
	return float64(contextTokens) / float64(agentRowMaxTokens)
}
