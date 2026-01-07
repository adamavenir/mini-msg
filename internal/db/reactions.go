package db

import (
	"database/sql"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// GetReactionsForMessage loads reactions from the fray_reactions table.
func GetReactionsForMessage(db *sql.DB, messageGUID string) (map[string][]types.ReactionEntry, error) {
	rows, err := db.Query(`
		SELECT emoji, agent_id, reacted_at
		FROM fray_reactions
		WHERE message_guid = ?
		ORDER BY reacted_at ASC
	`, messageGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reactions := make(map[string][]types.ReactionEntry)
	for rows.Next() {
		var emoji, agentID string
		var reactedAt int64
		if err := rows.Scan(&emoji, &agentID, &reactedAt); err != nil {
			return nil, err
		}
		reactions[emoji] = append(reactions[emoji], types.ReactionEntry{
			AgentID:   agentID,
			ReactedAt: reactedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reactions, nil
}

// GetReactionsForMessages loads reactions for multiple messages.
func GetReactionsForMessages(db *sql.DB, messageGUIDs []string) (map[string]map[string][]types.ReactionEntry, error) {
	if len(messageGUIDs) == 0 {
		return make(map[string]map[string][]types.ReactionEntry), nil
	}

	placeholders := make([]string, len(messageGUIDs))
	args := make([]any, len(messageGUIDs))
	for i, guid := range messageGUIDs {
		placeholders[i] = "?"
		args[i] = guid
	}

	query := `
		SELECT message_guid, emoji, agent_id, reacted_at
		FROM fray_reactions
		WHERE message_guid IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY reacted_at ASC
	`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string][]types.ReactionEntry)
	for rows.Next() {
		var msgGUID, emoji, agentID string
		var reactedAt int64
		if err := rows.Scan(&msgGUID, &emoji, &agentID, &reactedAt); err != nil {
			return nil, err
		}
		if result[msgGUID] == nil {
			result[msgGUID] = make(map[string][]types.ReactionEntry)
		}
		result[msgGUID][emoji] = append(result[msgGUID][emoji], types.ReactionEntry{
			AgentID:   agentID,
			ReactedAt: reactedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetMessagesReactedToByAgent returns messages that an agent has reacted to.
func GetMessagesReactedToByAgent(db *sql.DB, agentID string, limit int) ([]ReactionQueryResult, error) {
	query := `
		SELECT DISTINCT r.message_guid, r.emoji, r.reacted_at, m.from_agent, m.body, m.home
		FROM fray_reactions r
		INNER JOIN fray_messages m ON m.guid = r.message_guid
		WHERE r.agent_id = ? OR r.agent_id LIKE ?
		ORDER BY r.reacted_at DESC
	`
	args := []any{agentID, agentID + ".%"}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ReactionQueryResult
	for rows.Next() {
		var r ReactionQueryResult
		if err := rows.Scan(&r.MessageGUID, &r.Emoji, &r.ReactedAt, &r.FromAgent, &r.Body, &r.Home); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetMessagesWithReactionsFrom returns messages from an agent that have reactions.
func GetMessagesWithReactionsFrom(db *sql.DB, agentID string, limit int) ([]ReactionQueryResult, error) {
	query := `
		SELECT DISTINCT r.message_guid, r.emoji, r.reacted_at, r.agent_id, m.body, m.home
		FROM fray_reactions r
		INNER JOIN fray_messages m ON m.guid = r.message_guid
		WHERE m.from_agent = ? OR m.from_agent LIKE ?
		ORDER BY r.reacted_at DESC
	`
	args := []any{agentID, agentID + ".%"}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ReactionQueryResult
	for rows.Next() {
		var r ReactionQueryResult
		if err := rows.Scan(&r.MessageGUID, &r.Emoji, &r.ReactedAt, &r.ReactedBy, &r.Body, &r.Home); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ReactionQueryResult holds info about a reaction for display.
type ReactionQueryResult struct {
	MessageGUID string
	Emoji       string
	ReactedAt   int64
	FromAgent   string // who wrote the message
	ReactedBy   string // who reacted
	Body        string
	Home        string
}

// InsertReaction adds a reaction to the fray_reactions table.
func InsertReaction(db *sql.DB, messageGUID, agentID, emoji string, reactedAt int64) error {
	_, err := db.Exec(`
		INSERT INTO fray_reactions (message_guid, agent_id, emoji, reacted_at)
		VALUES (?, ?, ?, ?)
	`, messageGUID, agentID, emoji, reactedAt)
	return err
}

// InsertReactionNow adds a reaction with the current timestamp.
func InsertReactionNow(db *sql.DB, messageGUID, agentID, emoji string) (int64, error) {
	reactedAt := time.Now().UnixMilli()
	err := InsertReaction(db, messageGUID, agentID, emoji, reactedAt)
	return reactedAt, err
}

// Legacy helpers for backward compatibility with old JSON reactions format.
// These will be removed once migration is complete.

func normalizeReactionsLegacy(reactions map[string][]string) map[string][]string {
	if reactions == nil {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(reactions))
	for reaction, users := range reactions {
		reaction = strings.TrimSpace(reaction)
		if reaction == "" {
			continue
		}
		cleaned := dedupeNonEmpty(users)
		if len(cleaned) == 0 {
			continue
		}
		out[reaction] = cleaned
	}
	return out
}

func dedupeNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// ConvertLegacyReactions converts old map[string][]string format to new format.
// Since legacy format has no timestamps, uses 0 as placeholder.
func ConvertLegacyReactions(legacy map[string][]string) map[string][]types.ReactionEntry {
	if legacy == nil {
		return make(map[string][]types.ReactionEntry)
	}
	result := make(map[string][]types.ReactionEntry, len(legacy))
	for emoji, agents := range legacy {
		entries := make([]types.ReactionEntry, 0, len(agents))
		for _, agent := range agents {
			entries = append(entries, types.ReactionEntry{
				AgentID:   agent,
				ReactedAt: 0, // Legacy reactions have no timestamp
			})
		}
		result[emoji] = entries
	}
	return result
}

// ConvertToLegacyReactions converts new format back to old for JSONL compatibility during transition.
func ConvertToLegacyReactions(reactions map[string][]types.ReactionEntry) map[string][]string {
	if reactions == nil {
		return make(map[string][]string)
	}
	result := make(map[string][]string, len(reactions))
	for emoji, entries := range reactions {
		agents := make([]string, 0, len(entries))
		for _, entry := range entries {
			agents = append(agents, entry.AgentID)
		}
		result[emoji] = agents
	}
	return result
}
