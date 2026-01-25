package chat

import (
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleActivityPollMsg(msg activityPollMsg) (tea.Model, tea.Cmd) {
	// Fast poll for activity panel updates (250ms)
	m.animationFrame++ // Increment animation frame for spawn cycle animation
	if msg.managedAgents != nil {
		// Track presence changes for debouncing (suppress flicker)
		now := time.Now()
		const presenceDebounceMs = 1000 // 1 second debounce
		for i := range msg.managedAgents {
			agent := &msg.managedAgents[i]
			actualPresence, hasActual := m.agentActualPresence[agent.AgentID]
			if !hasActual {
				// First time seeing this agent - initialize both to current
				m.agentActualPresence[agent.AgentID] = agent.Presence
				m.agentDisplayPresence[agent.AgentID] = agent.Presence
				m.agentPresenceChanged[agent.AgentID] = now
			} else if agent.Presence != actualPresence {
				// Actual presence changed - record change time
				m.agentActualPresence[agent.AgentID] = agent.Presence
				m.agentPresenceChanged[agent.AgentID] = now

				// Display compact initiated event when agent starts compacting
				if agent.Presence == types.PresenceCompacting {
					event := newEventMessage(fmt.Sprintf("compact initiated for @%s", agent.AgentID))
					m.messages = append(m.messages, event)
					m.refreshViewport(true)
				}

				// Skip debounce for "wake up" transitions (inactive → active).
				// These should display immediately since they're deliberate, not flicker.
				fromInactive := actualPresence == types.PresenceOffline ||
					actualPresence == types.PresenceError ||
					actualPresence == types.PresenceBRB
				toActive := agent.Presence == types.PresenceSpawning ||
					agent.Presence == types.PresencePrompting ||
					agent.Presence == types.PresencePrompted ||
					agent.Presence == types.PresenceActive
				if fromInactive && toActive {
					m.agentDisplayPresence[agent.AgentID] = agent.Presence
				}
			}
			// Update display presence if debounce period has passed
			changedAt := m.agentPresenceChanged[agent.AgentID]
			if now.Sub(changedAt).Milliseconds() >= presenceDebounceMs {
				m.agentDisplayPresence[agent.AgentID] = m.agentActualPresence[agent.AgentID]
			}
		}
		m.managedAgents = msg.managedAgents
	}
	if msg.agentTokenUsage != nil {
		m.agentTokenUsage = msg.agentTokenUsage
	}
	// Detect daemon restart and trigger database refresh
	if msg.daemonStartedAt != 0 && msg.daemonStartedAt != m.daemonStartedAt {
		if m.daemonStartedAt != 0 {
			// Daemon restarted - reload messages to get fresh data
			m.status = "Daemon restarted, refreshing..."
			if err := m.reloadMessages(); err == nil {
				m.status = ""
			}
		}
		m.daemonStartedAt = msg.daemonStartedAt
	}
	return m, m.activityPollCmd()
}
