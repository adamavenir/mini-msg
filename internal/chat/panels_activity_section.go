package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

// renderActivitySection renders the activity panel section showing managed agents.
// Returns the rendered lines and the number of lines used.
func (m *Model) renderActivitySection(width int) ([]string, int) {
	if len(m.managedAgents) == 0 {
		return nil, 0
	}

	var lines []string
	recentOfflineThreshold := 4 * 60 * 60 // 4 hours in seconds (for recently offline agents)
	forkIdleThreshold := 5 * 60           // 5 minutes for fork sessions
	now := int64(0)
	if t := time.Now().Unix(); t > 0 {
		now = t
	}

	// Categorize agents into active/idle/offline, separating job workers
	var activeAgents, idleAgents, recentlyOfflineAgents []types.Agent
	offlineCount := 0                            // count of agents offline > 4h (hidden individually but shown in summary)
	jobWorkers := make(map[string][]types.Agent) // job_id -> workers

	for _, agent := range m.managedAgents {
		// Fork sessions (SessionMode is 3-char prefix, not "n" or "") hide after 5m idle
		isForkSession := agent.SessionMode != "" && agent.SessionMode != "n"
		if isForkSession {
			timeSinceActive := now - agent.LastSeen
			if agent.Presence == types.PresenceIdle || agent.Presence == types.PresenceOffline {
				if timeSinceActive > int64(forkIdleThreshold) {
					// Fork session idle > 5m: don't show in activity panel
					continue
				}
			}
		}

		// Determine if this is a job worker
		isJobWorker := agent.JobID != nil && *agent.JobID != ""

		// Categorize by presence state
		// Active: spawning, prompting, prompted, active, error
		// Idle: idle presence (has active session but idle)
		// Recently offline: offline within 4h
		// Offline: offline beyond 4h (hidden)
		category := ""
		if agent.Presence == types.PresenceSpawning || agent.Presence == types.PresencePrompting ||
			agent.Presence == types.PresencePrompted || agent.Presence == types.PresenceCompacting ||
			agent.Presence == types.PresenceActive || agent.Presence == types.PresenceError {
			category = "active"
		} else if agent.Presence == types.PresenceIdle {
			category = "idle"
		} else if agent.Presence == types.PresenceOffline {
			timeSinceActive := now - agent.LastSeen
			if timeSinceActive <= int64(recentOfflineThreshold) {
				category = "recently-offline"
			} else {
				category = "offline" // hidden
			}
		}

		if isJobWorker {
			jobWorkers[*agent.JobID] = append(jobWorkers[*agent.JobID], agent)
		} else if category == "active" {
			activeAgents = append(activeAgents, agent)
		} else if category == "idle" {
			idleAgents = append(idleAgents, agent)
		} else if category == "recently-offline" {
			recentlyOfflineAgents = append(recentlyOfflineAgents, agent)
		} else if category == "offline" {
			offlineCount++
		}
	}

	// Render regular active agents
	for _, agent := range activeAgents {
		line := m.renderAgentRow(agent, width)
		lines = append(lines, line)
	}

	// Render job worker clusters
	for jobID, workers := range jobWorkers {
		if m.expandedJobClusters[jobID] {
			// Expanded: show all workers
			for _, worker := range workers {
				line := m.renderAgentRow(worker, width)
				// Add indent for expanded workers
				lines = append(lines, "  "+line)
			}
		} else {
			// Collapsed: show cluster summary
			line := m.renderJobClusterRow(jobID, workers, width)
			lines = append(lines, line)
		}
	}

	// Render idle agents (have session but idle)
	for _, agent := range idleAgents {
		line := m.renderAgentRow(agent, width)
		lines = append(lines, line)
	}

	// Render recently offline agents (offline within 4h) with distinct visual
	for _, agent := range recentlyOfflineAgents {
		line := m.renderAgentRow(agent, width)
		// Apply italic grey styling to distinguish from idle
		styledLine := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true).Render(line)
		lines = append(lines, styledLine)
	}

	// Render offline summary (agents offline > 4h)
	if offlineCount > 0 {
		offlineLabel := fmt.Sprintf(" · %d offline", offlineCount)
		if width > 0 && len(offlineLabel) > width-1 {
			offlineLabel = offlineLabel[:width-1]
		}
		offlineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		lines = append(lines, offlineStyle.Render(offlineLabel))
	}

	return lines, len(lines)
}

// renderJobClusterRow renders a collapsed job worker cluster row.
// Format: "▶ baseAgent × count [suffix]"
func (m *Model) renderJobClusterRow(jobID string, workers []types.Agent, width int) string {
	if width <= 0 {
		width = 30
	}
	rowWidth := width - 3

	// Extract base agent name and suffix from first worker
	baseAgent := ""
	if len(workers) > 0 && workers[0].JobID != nil {
		// Worker ID format: baseAgent[suffix-idx]
		// Parse to extract base agent name
		workerID := workers[0].AgentID
		if idx := strings.Index(workerID, "["); idx > 0 {
			baseAgent = workerID[:idx]
		} else {
			baseAgent = workerID
		}
	}

	// Extract 4-char suffix from job ID (job-abc12345 -> abc1)
	suffix := ""
	if len(jobID) >= 8 {
		suffix = jobID[4:8]
	}

	// Count active vs total workers
	activeCount := 0
	for _, w := range workers {
		if w.Presence == types.PresenceActive || w.Presence == types.PresenceSpawning ||
			w.Presence == types.PresencePrompting || w.Presence == types.PresencePrompted ||
			w.Presence == types.PresenceCompacting {
			activeCount++
		}
	}

	// Determine dominant presence icon
	icon := "▶"
	if activeCount == 0 {
		icon = "▷" // all idle/offline
	}

	// Build label: "▶ baseAgent × count [suffix]"
	label := fmt.Sprintf(" %s %s × %d [%s]", icon, baseAgent, len(workers), suffix)

	// Truncate if needed
	if len(label) > rowWidth {
		label = label[:rowWidth]
	}

	// Style with agent color
	agentColor := m.colorForAgent(baseAgent)
	style := lipgloss.NewStyle().Foreground(agentColor).Bold(true)

	// Wrap in bubblezone for click handling
	zoneID := "job-cluster-" + jobID
	return m.zoneManager.Mark(zoneID, style.Render(label))
}
