package command

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func formatRelative(ts int64) string {
	now := time.Now().Unix()
	secondsAgo := now - ts
	if secondsAgo < 0 {
		return "just now"
	}
	if secondsAgo < 60 {
		return fmt.Sprintf("%ds ago", secondsAgo)
	}
	minutesAgo := secondsAgo / 60
	if minutesAgo < 60 {
		return fmt.Sprintf("%dm ago", minutesAgo)
	}
	hoursAgo := minutesAgo / 60
	if hoursAgo < 24 {
		return fmt.Sprintf("%dh ago", hoursAgo)
	}
	daysAgo := hoursAgo / 24
	if daysAgo < 7 {
		return fmt.Sprintf("%dd ago", daysAgo)
	}
	weeksAgo := daysAgo / 7
	return fmt.Sprintf("%dw ago", weeksAgo)
}

func isStale(lastSeen int64, staleHours int) bool {
	if staleHours <= 0 {
		return false
	}
	return lastSeen+int64(staleHours*3600) < time.Now().Unix()
}

func parseDuration(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("invalid duration")
	}
	unit := value[len(value)-1:]
	amountStr := value[:len(value)-1]
	amount, err := strconv.Atoi(amountStr)
	if err != nil || amount <= 0 {
		return 0, fmt.Errorf("invalid duration format: %s. Use 30m, 2h, or 1d", value)
	}

	switch unit {
	case "m":
		return int64(amount) * 60, nil
	case "h":
		return int64(amount) * 3600, nil
	case "d":
		return int64(amount) * 86400, nil
	default:
		return 0, fmt.Errorf("invalid duration format: %s. Use 30m, 2h, or 1d", value)
	}
}

func stripHash(value string) string {
	return strings.TrimPrefix(value, "#")
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func getMessageCounts(dbConn *sql.DB) (map[string]int64, error) {
	rows, err := dbConn.Query(`
		SELECT from_agent as agent_id, COUNT(*) as count
		FROM fray_messages
		GROUP BY from_agent
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var agentID string
		var count int64
		if err := rows.Scan(&agentID, &count); err != nil {
			return nil, err
		}
		counts[agentID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func getTotalMessageCount(dbConn *sql.DB) (int64, error) {
	row := dbConn.QueryRow("SELECT COUNT(*) FROM fray_messages")
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func getLastMessageTS(dbConn *sql.DB) (*int64, error) {
	row := dbConn.QueryRow("SELECT MAX(ts) FROM fray_messages")
	var ts sql.NullInt64
	if err := row.Scan(&ts); err != nil {
		return nil, err
	}
	if !ts.Valid {
		return nil, nil
	}
	value := ts.Int64
	return &value, nil
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func normalizeOptionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatOptionalValue(value *string) string {
	if value == nil || *value == "" {
		return "--"
	}
	return *value
}

func formatOptionalString(value string) string {
	if value == "" {
		return "--"
	}
	return value
}

func agentNicksForGUID(config *db.ProjectConfig, guid string) []string {
	if config == nil || len(config.KnownAgents) == 0 {
		return []string{}
	}
	entry, ok := config.KnownAgents[guid]
	if !ok || len(entry.Nicks) == 0 {
		return []string{}
	}
	nicks := make([]string, 0, len(entry.Nicks))
	for _, nick := range entry.Nicks {
		nick = strings.TrimSpace(nick)
		if nick == "" {
			continue
		}
		nicks = append(nicks, nick)
	}
	return nicks
}

func formatAgentLabel(agentID string, nicks []string) string {
	if len(nicks) == 0 {
		return "@" + agentID
	}
	formatted := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		if nick == "" {
			continue
		}
		formatted = append(formatted, "@"+nick)
	}
	if len(formatted) == 0 {
		return "@" + agentID
	}
	return fmt.Sprintf("@%s (aka %s)", agentID, strings.Join(formatted, ", "))
}

func filterEventMessages(messages []types.Message) []types.Message {
	filtered := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Type == types.MessageTypeEvent {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func filterDeletedMessages(messages []types.Message) []types.Message {
	filtered := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.ArchivedAt != nil && msg.Body == "[deleted]" {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

// subscribeAgentToThread subscribes an agent to a thread if not already subscribed.
// Source indicates how the subscription was triggered (post, mention, reply).
// For mentions, resolves to actual agents using prefix matching (e.g., "alice" -> "alice.1").
func subscribeAgentToThread(ctx *CommandContext, threadGUID, agentID string, at int64, source string) error {
	// Try exact match first
	agent, err := db.GetAgent(ctx.DB, agentID)
	if err != nil {
		return err
	}

	// If no exact match and this is a mention, try prefix matching
	if agent == nil && source == "mention" {
		agents, err := db.GetAgentsByPrefix(ctx.DB, agentID)
		if err != nil {
			return err
		}
		// Subscribe all matching agents
		for _, a := range agents {
			if a.LeftAt != nil {
				continue // Skip agents who have left
			}
			if err := subscribeAgentDirect(ctx, threadGUID, a.AgentID, at); err != nil {
				continue // Non-fatal for batch operations
			}
		}
		return nil
	}

	if agent == nil {
		// Agent doesn't exist, skip subscription
		return nil
	}

	if agent.LeftAt != nil {
		// Agent has left, skip subscription
		return nil
	}

	return subscribeAgentDirect(ctx, threadGUID, agentID, at)
}

// subscribeAgentDirect subscribes an agent without resolution.
func subscribeAgentDirect(ctx *CommandContext, threadGUID, agentID string, at int64) error {
	if err := db.SubscribeThread(ctx.DB, threadGUID, agentID, at); err != nil {
		return err
	}

	if err := db.AppendThreadSubscribe(ctx.Project.DBPath, db.ThreadSubscribeJSONLRecord{
		ThreadGUID:   threadGUID,
		AgentID:      agentID,
		SubscribedAt: at,
	}); err != nil {
		return err
	}

	return nil
}
