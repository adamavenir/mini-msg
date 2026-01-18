package command

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/adamavenir/fray/internal/usage"
	"github.com/spf13/cobra"
)

// TokenUsage holds token usage data for dashboard display.
type TokenUsage struct {
	SessionID   string
	TotalTokens int64
	InputTokens int64
	OutputTokens int64
}

// tokenCache caches transcript parsing results to avoid repeated file reads.
var tokenCache = struct {
	sync.RWMutex
	data map[string]tokenCacheEntry
}{data: make(map[string]tokenCacheEntry)}

type tokenCacheEntry struct {
	usage     *TokenUsage
	fetchedAt time.Time
}

const tokenCacheTTL = 30 * time.Second

// getTokenUsage fetches token usage for a session ID by parsing transcript files.
// Returns nil if session not found.
func getTokenUsage(sessionID string) *TokenUsage {
	if sessionID == "" {
		return nil
	}

	// Check cache
	tokenCache.RLock()
	if entry, ok := tokenCache.data[sessionID]; ok {
		if time.Since(entry.fetchedAt) < tokenCacheTTL {
			tokenCache.RUnlock()
			return entry.usage
		}
	}
	tokenCache.RUnlock()

	// Parse transcript directly
	sessionUsage, err := usage.GetSessionUsage(sessionID)
	if err != nil || sessionUsage == nil {
		// Cache the miss to avoid repeated failed calls
		tokenCache.Lock()
		tokenCache.data[sessionID] = tokenCacheEntry{usage: nil, fetchedAt: time.Now()}
		tokenCache.Unlock()
		return nil
	}

	tokenUsage := &TokenUsage{
		SessionID:    sessionID,
		InputTokens:  sessionUsage.InputTokens + sessionUsage.CachedTokens,
		OutputTokens: sessionUsage.OutputTokens,
		TotalTokens:  sessionUsage.InputTokens + sessionUsage.CachedTokens + sessionUsage.OutputTokens,
	}

	// Cache the result
	tokenCache.Lock()
	tokenCache.data[sessionID] = tokenCacheEntry{usage: tokenUsage, fetchedAt: time.Now()}
	tokenCache.Unlock()

	return tokenUsage
}

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

			// Get token usage for active agents (via ccusage)
			tokenUsage := make(map[string]*TokenUsage)
			for _, agent := range activeAgents {
				if agent.LastSessionID != nil && *agent.LastSessionID != "" {
					tokenUsage[agent.AgentID] = getTokenUsage(*agent.LastSessionID)
				}
			}

			// Get recent events (last 30 minutes)
			recentMessages, err := db.GetRecentMessages(ctx.DB, 30*60)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Filter to relevant events
			events := filterRelevantEvents(recentMessages)

			if ctx.JSONMode {
				return renderDashboardJSON(cmd, activeAgents, offlineAgents, unreadCounts, claimsByAgent, tokenUsage, events)
			}

			return renderDashboardText(cmd, activeAgents, offlineAgents, unreadCounts, claimsByAgent, tokenUsage, events)
		},
	}

	return cmd
}

// DashboardEvent represents a relevant event for the dashboard.
type DashboardEvent struct {
	Timestamp   int64
	AgentID     string
	EventType   string // "message", "status", "session_start", "session_end"
	Description string
}

func filterRelevantEvents(messages []types.Message) []DashboardEvent {
	var events []DashboardEvent

	for _, msg := range messages {
		// Include messages with @pm mentions (blocked/done/standup)
		if containsMention(msg.Mentions, "pm") {
			eventType := "message"
			if containsPrefix(msg.Body, "blocked:") {
				eventType = "blocked"
			} else if containsPrefix(msg.Body, "done:") {
				eventType = "done"
			}

			events = append(events, DashboardEvent{
				Timestamp:   msg.TS,
				AgentID:     msg.FromAgent,
				EventType:   eventType,
				Description: truncate(msg.Body, 60),
			})
			continue
		}

		// Include status messages (active:, blocked:, done:)
		if containsPrefix(msg.Body, "active:") ||
		   containsPrefix(msg.Body, "blocked:") ||
		   containsPrefix(msg.Body, "done:") {
			eventType := "status"
			if containsPrefix(msg.Body, "blocked:") {
				eventType = "blocked"
			} else if containsPrefix(msg.Body, "done:") {
				eventType = "done"
			}

			events = append(events, DashboardEvent{
				Timestamp:   msg.TS,
				AgentID:     msg.FromAgent,
				EventType:   eventType,
				Description: truncate(msg.Body, 60),
			})
		}
	}

	return events
}

func containsMention(mentions []string, agent string) bool {
	for _, m := range mentions {
		if m == agent {
			return true
		}
	}
	return false
}

func containsPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func renderDashboardText(cmd *cobra.Command, activeAgents, offlineAgents []types.Agent, unreadCounts map[string]int, claimsByAgent map[string][]types.Claim, tokenUsage map[string]*TokenUsage, events []DashboardEvent) error {
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

			// Token usage
			tokensStr := formatTokenUsage(tokenUsage[agent.AgentID])

			fmt.Fprintf(out, "  %-12s [%-20s]  %-40s  %-12s  %s\n",
				agent.AgentID, statusStr, workStr, tokensStr, unreadStr)
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

	// Recent events section
	fmt.Fprintln(out, "Recent Events (last 30m):")
	if len(events) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		now := time.Now().Unix()
		for _, event := range events {
			relTime := formatRelativeTime(now - event.Timestamp)
			fmt.Fprintf(out, "  %8s - @%s: %s\n", relTime, event.AgentID, event.Description)
		}
	}
	fmt.Fprintln(out)

	fmt.Fprintf(out, "Last updated: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	return nil
}

func renderDashboardJSON(cmd *cobra.Command, activeAgents, offlineAgents []types.Agent, unreadCounts map[string]int, claimsByAgent map[string][]types.Claim, tokenUsage map[string]*TokenUsage, events []DashboardEvent) error {
	now := time.Now()

	payload := map[string]any{
		"timestamp":      now.Format(time.RFC3339),
		"active_agents":  buildActiveAgentsPayload(activeAgents, unreadCounts, claimsByAgent, tokenUsage),
		"offline_agents": buildOfflineAgentsPayload(offlineAgents, now.Unix()),
		"recent_events":  buildEventsPayload(events),
	}

	return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
}

func buildActiveAgentsPayload(agents []types.Agent, unreadCounts map[string]int, claimsByAgent map[string][]types.Claim, tokenUsage map[string]*TokenUsage) []map[string]any {
	result := make([]map[string]any, 0, len(agents))
	for _, agent := range agents {
		entry := map[string]any{
			"agent_id":      agent.AgentID,
			"status":        string(agent.Presence),
			"unread_count":  unreadCounts[agent.AgentID],
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

		// Token usage
		if u := tokenUsage[agent.AgentID]; u != nil {
			entry["tokens"] = map[string]any{
				"total_tokens":  u.TotalTokens,
				"input_tokens":  u.InputTokens,
				"output_tokens": u.OutputTokens,
				"session_id":    u.SessionID,
			}
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

func buildEventsPayload(events []DashboardEvent) []map[string]any {
	result := make([]map[string]any, 0, len(events))
	for _, event := range events {
		result = append(result, map[string]any{
			"timestamp": time.Unix(event.Timestamp, 0).Format(time.RFC3339),
			"agent_id": event.AgentID,
			"event_type": event.EventType,
			"description": event.Description,
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
	case types.PresencePrompting:
		statusParts = append(statusParts, "prompting")
	case types.PresencePrompted:
		statusParts = append(statusParts, "prompted")
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

func formatTokenUsage(u *TokenUsage) string {
	if u == nil {
		return "N/A"
	}
	// Format total tokens in k/M format
	if u.TotalTokens >= 1000000 {
		return fmt.Sprintf("%.1fM tok", float64(u.TotalTokens)/1000000)
	}
	if u.TotalTokens >= 1000 {
		return fmt.Sprintf("%.0fk tok", float64(u.TotalTokens)/1000)
	}
	return fmt.Sprintf("%d tok", u.TotalTokens)
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

func formatRelativeTime(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1d ago"
	}
	return fmt.Sprintf("%dd ago", days)
}
