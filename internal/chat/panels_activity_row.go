package chat

import "github.com/adamavenir/fray/internal/types"

// renderAgentRow renders a single agent row with token usage progress bar.
// Design:
// - Icon + agent name: bold, in agent's color
// - Status: italic, dim grey
// - Background: black (used/progress) | sidebar default (unused)
// - Danger zone (>80%): red (used) | sidebar default (unused), white text
// - Offline: no background, grey icon + agent name
func (m *Model) renderAgentRow(agent types.Agent, width int) string {
	if width <= 0 {
		width = 30
	}
	rowWidth := width - 3 // extra padding from sidebar edge

	// Use debounced display presence to suppress flicker from rapid state changes
	displayPresence := m.agentRowDisplayPresence(agent)
	icon, displayStatus, statusDisplay := agentRowStatus(agent, displayPresence, m.animationFrame, m.statusInvoker)
	unread := m.agentUnreadCounts[agent.AgentID]
	content := buildAgentRowContent(agent, icon, displayStatus, unread, rowWidth)
	agentColor := m.colorForAgent(agent.AgentID)
	tokenPercent := agentRowTokenPercent(m.agentTokenUsage[agent.AgentID])
	style := agentRowStyleFor(displayPresence, agent.SessionMode, statusDisplay, agentColor, tokenPercent, rowWidth)
	rowText := renderAgentRowText(content, style)

	// Wrap in bubblezone for click handling
	zoneID := "agent-" + agent.AgentID
	return m.zoneManager.Mark(zoneID, rowText)
}
