package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewDashboardCmd creates the dashboard command.
func NewDashboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Show agent status dashboard",
		Long:  "Display comprehensive agent status dashboard with active/offline agents, token usage, and recent events",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			// Get managed agents (only those controlled by daemon)
			agents, err := db.GetManagedAgents(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Separate into active and offline
			staleThreshold := 4 * time.Hour
			now := time.Now().Unix()
			var activeAgents, offlineAgents []types.Agent
			for _, agent := range agents {
				if agent.LeftAt != nil {
					offlineAgents = append(offlineAgents, agent)
					continue
				}
				if agent.Presence == types.PresenceOffline {
					offlineAgents = append(offlineAgents, agent)
					continue
				}
				timeSinceActive := time.Duration(now-agent.LastSeen) * time.Second
				if timeSinceActive > staleThreshold {
					offlineAgents = append(offlineAgents, agent)
				} else {
					activeAgents = append(activeAgents, agent)
				}
			}

			// Get unread counts for each agent
			unreadCounts := make(map[string]int)
			for _, agent := range activeAgents {
				count, err := db.GetUnreadCountForAgent(ctx.DB, agent.AgentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				unreadCounts[agent.AgentID] = count
			}

			// Get claims for each agent to show current work
			allClaims, err := db.GetAllClaims(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			claimsByAgent := make(map[string][]types.Claim)
			for _, claim := range allClaims {
				claimsByAgent[claim.AgentID] = append(claimsByAgent[claim.AgentID], claim)
			}

			if ctx.JSONMode {
				return renderDashboardJSON(cmd, activeAgents, offlineAgents, unreadCounts, claimsByAgent)
			}

			return renderDashboardText(cmd, activeAgents, offlineAgents, unreadCounts, claimsByAgent)
		},
	}

	return cmd
}

func renderDashboardText(cmd *cobra.Command, activeAgents, offlineAgents []types.Agent, unreadCounts map[string]int, claimsByAgent map[string][]types.Claim) error {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "FRAY DASHBOARD")
	fmt.Fprintln(out)

	// Active agents section
	fmt.Fprintf(out, "Active Agents (%d):\n", len(activeAgents))
	if len(activeAgents) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, agent := range activeAgents {
			// Status and session phase
			statusStr := formatAgentStatus(agent)

			// Current work from claims or last thread
			workStr := formatCurrentWork(agent, claimsByAgent[agent.AgentID])

			// Unread count
			unread := unreadCounts[agent.AgentID]
			unreadStr := fmt.Sprintf("%d unread", unread)

			fmt.Fprintf(out, "  %-12s [%-20s]  %-40s  %s\n",
				agent.AgentID, statusStr, workStr, unreadStr)
		}
	}
	fmt.Fprintln(out)

	// Offline agents section
	fmt.Fprintf(out, "Offline (%d):\n", len(offlineAgents))
	if len(offlineAgents) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		now := time.Now().Unix()
		for _, agent := range offlineAgents {
			offlineTime := time.Duration(now-agent.LastSeen) * time.Second
			fmt.Fprintf(out, "  %-12s (offline %s)\n", agent.AgentID, formatDuration(offlineTime))
		}
	}
	fmt.Fprintln(out)

	// Recent events placeholder
	fmt.Fprintln(out, "Recent Events (last 30m):")
	fmt.Fprintln(out, "  (coming soon)")
	fmt.Fprintln(out)

	fmt.Fprintf(out, "Last updated: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	return nil
}

func renderDashboardJSON(cmd *cobra.Command, activeAgents, offlineAgents []types.Agent, unreadCounts map[string]int, claimsByAgent map[string][]types.Claim) error {
	now := time.Now()

	payload := map[string]any{
		"timestamp": now.Format(time.RFC3339),
		"active_agents": buildActiveAgentsPayload(activeAgents, unreadCounts, claimsByAgent),
		"offline_agents": buildOfflineAgentsPayload(offlineAgents, now.Unix()),
		"recent_events": []any{}, // TODO: implement in fray-h0ro
	}

	return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
}

func buildActiveAgentsPayload(agents []types.Agent, unreadCounts map[string]int, claimsByAgent map[string][]types.Claim) []map[string]any {
	result := make([]map[string]any, 0, len(agents))
	for _, agent := range agents {
		entry := map[string]any{
			"agent_id": agent.AgentID,
			"status": string(agent.Presence),
			"unread_count": unreadCounts[agent.AgentID],
			"last_activity": time.Unix(agent.LastSeen, 0).Format(time.RFC3339),
		}

		if agent.Status != nil {
			entry["status_message"] = *agent.Status
		}

		// Extract session phase if present
		if phase := extractSessionPhase(agent); phase != "" {
			entry["session_phase"] = phase
		}

		// Current work from claims
		claims := claimsByAgent[agent.AgentID]
		if len(claims) > 0 {
			claimsList := make([]string, 0, len(claims))
			for _, claim := range claims {
				switch claim.ClaimType {
				case types.ClaimTypeFile:
					claimsList = append(claimsList, claim.Pattern)
				case types.ClaimTypeBD:
					claimsList = append(claimsList, "bd:"+claim.Pattern)
				case types.ClaimTypeIssue:
					claimsList = append(claimsList, "issue:"+claim.Pattern)
				}
			}
			entry["claims"] = claimsList
		}

		result = append(result, entry)
	}
	return result
}

func buildOfflineAgentsPayload(agents []types.Agent, now int64) []map[string]any {
	result := make([]map[string]any, 0, len(agents))
	for _, agent := range agents {
		offlineDuration := time.Duration(now-agent.LastSeen) * time.Second
		result = append(result, map[string]any{
			"agent_id": agent.AgentID,
			"offline_duration_h": offlineDuration.Hours(),
		})
	}
	return result
}

func formatAgentStatus(agent types.Agent) string {
	statusParts := []string{}

	// Presence state
	switch agent.Presence {
	case types.PresenceActive:
		statusParts = append(statusParts, "active")
	case types.PresenceSpawning:
		statusParts = append(statusParts, "spawning")
	case types.PresenceIdle:
		statusParts = append(statusParts, "idle")
	case types.PresenceError:
		statusParts = append(statusParts, "error")
	default:
		statusParts = append(statusParts, "active")
	}

	// Session phase
	if phase := extractSessionPhase(agent); phase != "" {
		statusParts = append(statusParts, phase)
	}

	return fmt.Sprintf("%s", statusParts[0])
}

func extractSessionPhase(agent types.Agent) string {
	if agent.Status == nil {
		return ""
	}

	// Look for /fly, /hop, /land in status
	status := *agent.Status
	if contains(status, "/fly") {
		return "/fly"
	}
	if contains(status, "/hop") {
		return "/hop"
	}
	if contains(status, "/land") {
		return "/land"
	}

	return ""
}

func formatCurrentWork(agent types.Agent, claims []types.Claim) string {
	// First try to get work from claims
	if len(claims) > 0 {
		claim := claims[0]
		switch claim.ClaimType {
		case types.ClaimTypeBD:
			return claim.Pattern + ": " + truncate(claim.Pattern, 30)
		case types.ClaimTypeFile:
			return "Working on " + truncate(claim.Pattern, 30)
		case types.ClaimTypeIssue:
			return "Issue #" + claim.Pattern
		}
	}

	// Fall back to status message
	if agent.Status != nil && *agent.Status != "" {
		return truncate(*agent.Status, 40)
	}

	return "(no current work)"
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "< 1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		   len(s) > len(substr) && s[len(s)-len(substr):] == substr ||
		   findInString(s, substr)
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
