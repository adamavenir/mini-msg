package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
	"github.com/gobwas/glob"
	"modernc.org/sqlite"
)

const (
	sqliteConstraint       = 19
	sqliteConstraintUnique = 2067
)

// AgentUpdates represents partial agent updates.
type AgentUpdates struct {
	Status   types.OptionalString
	Purpose  types.OptionalString
	LastSeen types.OptionalInt64
	LeftAt   types.OptionalInt64
}

// GetAgent returns an agent by exact ID.
func GetAgent(db *sql.DB, agentID string) (*types.Agent, error) {
	row := db.QueryRow(`
		SELECT guid, agent_id, status, purpose, registered_at, last_seen, left_at
		FROM fray_agents
		WHERE agent_id = ?
	`, agentID)

	agent, err := scanAgent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

// GetAgentsByPrefix returns agents matching a prefix.
func GetAgentsByPrefix(db *sql.DB, prefix string) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, registered_at, last_seen, left_at
		FROM fray_agents
		WHERE agent_id = ? OR agent_id LIKE ?
		ORDER BY agent_id
	`, prefix, fmt.Sprintf("%s.%%", prefix))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []types.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetAgents returns all agents.
func GetAgents(db *sql.DB) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, registered_at, last_seen, left_at
		FROM fray_agents
		ORDER BY agent_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []types.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

// CreateAgent inserts a new agent.
func CreateAgent(db *sql.DB, agent types.Agent) error {
	guid := agent.GUID
	if guid == "" {
		var err error
		guid, err = generateUniqueGUIDForTable(db, "fray_agents", "usr")
		if err != nil {
			return err
		}
	}

	_, err := db.Exec(`
		INSERT INTO fray_agents (guid, agent_id, status, purpose, registered_at, last_seen, left_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, guid, agent.AgentID, agent.Status, agent.Purpose, agent.RegisteredAt, agent.LastSeen, agent.LeftAt)
	return err
}

// UpdateAgent updates agent fields.
func UpdateAgent(db *sql.DB, agentID string, updates AgentUpdates) error {
	var fields []string
	var args []any

	if updates.Status.Set {
		fields = append(fields, "status = ?")
		args = append(args, nullableValue(updates.Status.Value))
	}
	if updates.Purpose.Set {
		fields = append(fields, "purpose = ?")
		args = append(args, nullableValue(updates.Purpose.Value))
	}
	if updates.LastSeen.Set {
		fields = append(fields, "last_seen = ?")
		args = append(args, nullableValue(updates.LastSeen.Value))
	}
	if updates.LeftAt.Set {
		fields = append(fields, "left_at = ?")
		args = append(args, nullableValue(updates.LeftAt.Value))
	}

	if len(fields) == 0 {
		return nil
	}

	args = append(args, agentID)
	query := fmt.Sprintf("UPDATE fray_agents SET %s WHERE agent_id = ?", strings.Join(fields, ", "))
	_, err := db.Exec(query, args...)
	return err
}

// GetActiveAgents returns non-stale agents.
func GetActiveAgents(db *sql.DB, staleHours int) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, registered_at, last_seen, left_at
		FROM fray_agents
		WHERE left_at IS NULL
		  AND last_seen > (strftime('%s', 'now') - ? * 3600)
		ORDER BY last_seen DESC
	`, staleHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []types.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetAllAgents returns all agents.
func GetAllAgents(db *sql.DB) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, registered_at, last_seen, left_at
		FROM fray_agents
		ORDER BY agent_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []types.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetActiveUsers returns active chat users.
func GetActiveUsers(db *sql.DB) ([]string, error) {
	username, err := GetConfig(db, "username")
	if err != nil {
		return nil, err
	}
	if username == "" {
		return nil, nil
	}
	return []string{username}, nil
}

// GetAgentBases returns unique base agent IDs.
func GetAgentBases(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query(`SELECT agent_id FROM fray_agents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bases := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		lastDot := strings.LastIndex(id, ".")
		if lastDot != -1 {
			suffix := id[lastDot+1:]
			if isNumericSuffix(suffix) {
				bases[id[:lastDot]] = struct{}{}
				continue
			}
		}
		bases[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bases, nil
}

// IsAgentActive reports whether an agent is active.
func IsAgentActive(db *sql.DB, agentID string, staleHours int) (bool, error) {
	row := db.QueryRow(`
		SELECT 1 FROM fray_agents
		WHERE agent_id = ?
		  AND left_at IS NULL
		  AND last_seen > (strftime('%s', 'now') - ? * 3600)
	`, agentID, staleHours)
	var exists int
	err := row.Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RenameAgent updates IDs across tables.
func RenameAgent(db *sql.DB, oldID, newID string) error {
	oldAgent, err := GetAgent(db, oldID)
	if err != nil {
		return err
	}
	if oldAgent == nil {
		return fmt.Errorf("agent not found: %s", oldID)
	}

	existing, err := GetAgent(db, newID)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("agent already exists: %s", newID)
	}

	if _, err := db.Exec("UPDATE fray_agents SET agent_id = ? WHERE agent_id = ?", newID, oldID); err != nil {
		return err
	}
	if _, err := db.Exec("UPDATE fray_messages SET from_agent = ? WHERE from_agent = ?", newID, oldID); err != nil {
		return err
	}

	rows, err := db.Query(`
		SELECT guid, mentions FROM fray_messages
		WHERE mentions LIKE ?
	`, fmt.Sprintf("%%\"%s\"%%", oldID))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var guid string
		var mentionsJSON string
		if err := rows.Scan(&guid, &mentionsJSON); err != nil {
			return err
		}

		var mentions []string
		if err := json.Unmarshal([]byte(mentionsJSON), &mentions); err != nil {
			return err
		}

		updated := false
		for i, mention := range mentions {
			if mention == oldID {
				mentions[i] = newID
				updated = true
			}
		}
		if !updated {
			continue
		}
		updatedJSON, err := json.Marshal(mentions)
		if err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE fray_messages SET mentions = ? WHERE guid = ?", string(updatedJSON), guid); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if err := updateReactionsForAgent(db, oldID, newID); err != nil {
		return err
	}

	return UpdateClaimsAgentID(db, oldID, newID)
}

// MergeAgentHistory reassigns message history and claims from one agent to another.
func MergeAgentHistory(db *sql.DB, fromID, toID string) (int64, error) {
	if fromID == "" || toID == "" {
		return 0, fmt.Errorf("agent ids are required")
	}
	if fromID == toID {
		return 0, fmt.Errorf("cannot merge an agent into itself")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	updated, err := mergeAgentHistoryWith(tx, fromID, toID)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

func mergeAgentHistoryWith(tx *sql.Tx, fromID, toID string) (int64, error) {
	result, err := tx.Exec("UPDATE fray_messages SET from_agent = ? WHERE from_agent = ?", toID, fromID)
	if err != nil {
		return 0, err
	}
	messageCount, _ := result.RowsAffected()

	rows, err := tx.Query(`
		SELECT guid, mentions FROM fray_messages
		WHERE mentions LIKE ?
	`, fmt.Sprintf("%%\"%s\"%%", fromID))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var guid string
		var mentionsJSON string
		if err := rows.Scan(&guid, &mentionsJSON); err != nil {
			return 0, err
		}

		var mentions []string
		if err := json.Unmarshal([]byte(mentionsJSON), &mentions); err != nil {
			return 0, err
		}

		updated := false
		for i, mention := range mentions {
			if mention == fromID {
				mentions[i] = toID
				updated = true
			}
		}
		if !updated {
			continue
		}
		mentions = dedupeStrings(mentions)
		updatedJSON, err := json.Marshal(mentions)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec("UPDATE fray_messages SET mentions = ? WHERE guid = ?", string(updatedJSON), guid); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if err := updateReactionsForAgent(tx, fromID, toID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec("UPDATE fray_claims SET agent_id = ? WHERE agent_id = ?", toID, fromID); err != nil {
		return 0, err
	}

	return messageCount, nil
}

func updateReactionsForAgent(db DBTX, oldID, newID string) error {
	rows, err := db.Query(`
		SELECT guid, reactions FROM fray_messages
		WHERE reactions LIKE ?
	`, fmt.Sprintf("%%\"%s\"%%", oldID))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var guid string
		var reactionsJSON string
		if err := rows.Scan(&guid, &reactionsJSON); err != nil {
			return err
		}

		var reactions map[string][]string
		if err := json.Unmarshal([]byte(reactionsJSON), &reactions); err != nil {
			return err
		}

		updated := false
		for reaction, users := range reactions {
			next := make([]string, 0, len(users))
			for _, user := range users {
				if user == oldID {
					user = newID
					updated = true
				}
				next = append(next, user)
			}
			reactions[reaction] = dedupeNonEmpty(next)
		}
		if !updated {
			continue
		}
		reactions = normalizeReactions(reactions)
		updatedJSON, err := json.Marshal(reactions)
		if err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE fray_messages SET reactions = ? WHERE guid = ?", string(updatedJSON), guid); err != nil {
			return err
		}
	}
	return rows.Err()
}

// GetMaxVersion returns the highest version for a base name.
func GetMaxVersion(db *sql.DB, base string) (int, error) {
	pattern := fmt.Sprintf("%s.[0-9]*", base)
	rows, err := db.Query("SELECT agent_id FROM fray_agents WHERE agent_id GLOB ?", pattern)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	maxVersion := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		parts := strings.Split(id, ".")
		if len(parts) == 0 {
			continue
		}
		version := parseNumeric(parts[len(parts)-1])
		if version > maxVersion {
			maxVersion = version
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return maxVersion, nil
}

// NextVersion returns the next version for a base name.
func NextVersion(db *sql.DB, base string) (int, error) {
	max, err := GetMaxVersion(db, base)
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

// CreateMessage inserts a new message.
func CreateMessage(db *sql.DB, message types.Message) (types.Message, error) {
	ts := message.TS
	if ts == 0 {
		ts = time.Now().Unix()
	}

	channelID := message.ChannelID
	if channelID == nil {
		value, err := GetConfig(db, "channel_id")
		if err != nil {
			return types.Message{}, err
		}
		if value != "" {
			channelID = &value
		}
	}

	mentionsJSON, err := json.Marshal(message.Mentions)
	if err != nil {
		return types.Message{}, err
	}
	reactionsJSON, err := json.Marshal(normalizeReactions(message.Reactions))
	if err != nil {
		return types.Message{}, err
	}

	msgType := message.Type
	if msgType == "" {
		msgType = types.MessageTypeAgent
	}

	guid, err := generateUniqueGUIDForTable(db, "fray_messages", "msg")
	if err != nil {
		return types.Message{}, err
	}

	home := message.Home
	if home == "" {
		home = "room"
	}

	_, err = db.Exec(`
		INSERT INTO fray_messages (guid, ts, channel_id, home, from_agent, body, mentions, type, "references", surface_message, reply_to, edited_at, archived_at, reactions)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)
	`, guid, ts, channelID, home, message.FromAgent, message.Body, string(mentionsJSON), msgType, message.References, message.SurfaceMessage, message.ReplyTo, string(reactionsJSON))
	if err != nil {
		return types.Message{}, err
	}

	return types.Message{
		ID:             guid,
		TS:             ts,
		ChannelID:      channelID,
		Home:           home,
		FromAgent:      message.FromAgent,
		Body:           message.Body,
		Mentions:       message.Mentions,
		Reactions:      normalizeReactions(message.Reactions),
		Type:           msgType,
		References:     message.References,
		SurfaceMessage: message.SurfaceMessage,
		ReplyTo:        message.ReplyTo,
		EditedAt:       nil,
		ArchivedAt:     nil,
	}, nil
}

// GetMessages returns messages in chronological order.
func GetMessages(db *sql.DB, options *types.MessageQueryOptions) ([]types.Message, error) {
	var sinceCursor, beforeCursor *types.MessageCursor
	var err error

	if options != nil {
		sinceCursor, err = resolveCursor(db, options.Since, options.SinceID)
		if err != nil {
			return nil, err
		}
		if (options.Since != nil || options.SinceID != "") && sinceCursor == nil {
			return nil, fmt.Errorf("message not found: %s", options.SinceID)
		}

		beforeCursor, err = resolveCursor(db, options.Before, options.BeforeID)
		if err != nil {
			return nil, err
		}
		if (options.Before != nil || options.BeforeID != "") && beforeCursor == nil {
			return nil, fmt.Errorf("message not found: %s", options.BeforeID)
		}
	}

	limit := 0
	includeArchived := false
	filter := (*types.Filter)(nil)
	home := "room"
	if options != nil {
		limit = options.Limit
		includeArchived = options.IncludeArchived
		filter = options.Filter
		if options.Home != nil {
			if *options.Home == "" {
				home = ""
			} else {
				home = *options.Home
			}
		}
	}

	if limit > 0 && sinceCursor == nil && beforeCursor == nil {
		var conditions []string
		var params []any

		if !includeArchived {
			conditions = append(conditions, "archived_at IS NULL")
		}

		if home != "" {
			conditions = append(conditions, "home = ?")
			params = append(params, home)
		}

		if clause, args := buildFilterCondition(filter); clause != "" {
			conditions = append(conditions, clause)
			params = append(params, args...)
		}

		whereClause := ""
		if len(conditions) > 0 {
			whereClause = " WHERE " + strings.Join(conditions, " AND ")
		}

		query := fmt.Sprintf(`
			SELECT * FROM (
				SELECT * FROM fray_messages%s
				ORDER BY ts DESC, guid DESC
				LIMIT ?
			) ORDER BY ts ASC, guid ASC
		`, whereClause)
		params = append(params, limit)

		rows, err := db.Query(query, params...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		return scanMessages(rows)
	}

	query := "SELECT * FROM fray_messages"
	var conditions []string
	var params []any

	if !includeArchived {
		conditions = append(conditions, "archived_at IS NULL")
	}

	if home != "" {
		conditions = append(conditions, "home = ?")
		params = append(params, home)
	}

	if sinceCursor != nil {
		clause, args := buildCursorCondition("", ">", sinceCursor)
		conditions = append(conditions, clause)
		params = append(params, args...)
	}

	if beforeCursor != nil {
		clause, args := buildCursorCondition("", "<", beforeCursor)
		conditions = append(conditions, clause)
		params = append(params, args...)
	}

	if clause, args := buildFilterCondition(filter); clause != "" {
		conditions = append(conditions, clause)
		params = append(params, args...)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY ts ASC, guid ASC"
	if limit > 0 {
		query += " LIMIT ?"
		params = append(params, limit)
	}

	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows)
}

// GetMessagesWithMention returns messages mentioning an agent prefix.
func GetMessagesWithMention(db *sql.DB, mentionPrefix string, options *types.MessageQueryOptions) ([]types.Message, error) {
	var sinceCursor, beforeCursor *types.MessageCursor
	var err error

	if options != nil {
		sinceCursor, err = resolveCursor(db, options.Since, options.SinceID)
		if err != nil {
			return nil, err
		}
		if (options.Since != nil || options.SinceID != "") && sinceCursor == nil {
			return nil, fmt.Errorf("message not found: %s", options.SinceID)
		}

		beforeCursor, err = resolveCursor(db, options.Before, options.BeforeID)
		if err != nil {
			return nil, err
		}
		if (options.Before != nil || options.BeforeID != "") && beforeCursor == nil {
			return nil, fmt.Errorf("message not found: %s", options.BeforeID)
		}
	}

	filterUnread := false
	agentPrefix := mentionPrefix
	includeArchived := false
	limit := 0
	home := "room"
	if options != nil {
		filterUnread = options.UnreadOnly
		if options.AgentPrefix != "" {
			agentPrefix = options.AgentPrefix
		}
		includeArchived = options.IncludeArchived
		limit = options.Limit
		if options.Home != nil {
			if *options.Home == "" {
				home = ""
			} else {
				home = *options.Home
			}
		}
	}

	query := `
		SELECT DISTINCT m.* FROM fray_messages m, json_each(m.mentions) j
	`
	var params []any

	if filterUnread {
		query += `
		LEFT JOIN fray_read_receipts r
		  ON m.guid = r.message_guid AND r.agent_prefix = ?
		`
		params = append(params, agentPrefix)
	}

	query += `
		WHERE (j.value = 'all' OR j.value = ? OR j.value LIKE ?)
	`
	params = append(params, mentionPrefix, fmt.Sprintf("%s.%%", mentionPrefix))

	var conditions []string
	if !includeArchived {
		conditions = append(conditions, "m.archived_at IS NULL")
	}
	if home != "" {
		conditions = append(conditions, "m.home = ?")
		params = append(params, home)
	}
	if filterUnread {
		conditions = append(conditions, "r.message_guid IS NULL")
	}
	if sinceCursor != nil {
		clause, args := buildCursorCondition("m.", ">", sinceCursor)
		conditions = append(conditions, clause)
		params = append(params, args...)
	}
	if beforeCursor != nil {
		clause, args := buildCursorCondition("m.", "<", beforeCursor)
		conditions = append(conditions, clause)
		params = append(params, args...)
	}

	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY m.ts ASC, m.guid ASC"
	if limit > 0 {
		query += " LIMIT ?"
		params = append(params, limit)
	}

	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows)
}

// GetLastMessageCursor returns the last message cursor.
func GetLastMessageCursor(db *sql.DB) (*types.MessageCursor, error) {
	row := db.QueryRow(`
		SELECT guid, ts FROM fray_messages
		WHERE home = 'room'
		ORDER BY ts DESC, guid DESC
		LIMIT 1
	`)
	var cursor types.MessageCursor
	if err := row.Scan(&cursor.GUID, &cursor.TS); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &cursor, nil
}

// GetMessage returns a message by GUID.
func GetMessage(db *sql.DB, messageID string) (*types.Message, error) {
	row := db.QueryRow("SELECT * FROM fray_messages WHERE guid = ?", messageID)
	message, err := scanMessage(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &message, nil
}

// GetMessageByPrefix returns a message by GUID prefix.
func GetMessageByPrefix(db *sql.DB, prefix string) (*types.Message, error) {
	normalized := prefix
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(lower, "msg-") {
		normalized = normalized[4:]
	}

	rows, err := db.Query(`
		SELECT * FROM fray_messages
		WHERE guid LIKE ?
		ORDER BY ts DESC
		LIMIT 2
	`, fmt.Sprintf("msg-%s%%", normalized))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(messages) == 1 {
		return &messages[0], nil
	}
	return nil, nil
}

// AddReaction adds a reaction for a message.
func AddReaction(db *sql.DB, messageID, reactor, reaction string) (*types.Message, bool, error) {
	msg, err := GetMessage(db, messageID)
	if err != nil {
		return nil, false, err
	}
	if msg == nil {
		return nil, false, fmt.Errorf("message %s not found", messageID)
	}

	reactions := cloneReactions(msg.Reactions)
	users := append([]string{}, reactions[reaction]...)
	if containsString(users, reactor) {
		return msg, false, nil
	}
	users = append(users, reactor)
	reactions[reaction] = users
	reactions = normalizeReactions(reactions)

	reactionsJSON, err := json.Marshal(reactions)
	if err != nil {
		return nil, false, err
	}
	if _, err := db.Exec("UPDATE fray_messages SET reactions = ? WHERE guid = ?", string(reactionsJSON), messageID); err != nil {
		return nil, false, err
	}

	updated, err := GetMessage(db, messageID)
	if err != nil {
		return nil, false, err
	}
	if updated == nil {
		return nil, false, fmt.Errorf("message %s not found", messageID)
	}
	return updated, true, nil
}

// RemoveReactions removes a reactor from all reactions on a message.
func RemoveReactions(db *sql.DB, messageID, reactor string) (*types.Message, bool, error) {
	msg, err := GetMessage(db, messageID)
	if err != nil {
		return nil, false, err
	}
	if msg == nil {
		return nil, false, fmt.Errorf("message %s not found", messageID)
	}

	removed := false
	reactions := cloneReactions(msg.Reactions)
	next := map[string][]string{}
	for reaction, users := range reactions {
		cleaned, didRemove := removeString(users, reactor)
		if didRemove {
			removed = true
		}
		if len(cleaned) == 0 {
			continue
		}
		next[reaction] = cleaned
	}
	next = normalizeReactions(next)

	if !removed {
		return msg, false, nil
	}

	reactionsJSON, err := json.Marshal(next)
	if err != nil {
		return nil, false, err
	}
	if _, err := db.Exec("UPDATE fray_messages SET reactions = ? WHERE guid = ?", string(reactionsJSON), messageID); err != nil {
		return nil, false, err
	}

	updated, err := GetMessage(db, messageID)
	if err != nil {
		return nil, false, err
	}
	if updated == nil {
		return nil, false, fmt.Errorf("message %s not found", messageID)
	}
	return updated, true, nil
}

// GetMessageReactions returns reactions for the given message IDs.
func GetMessageReactions(db *sql.DB, messageIDs []string) (map[string]map[string][]string, error) {
	if len(messageIDs) == 0 {
		return map[string]map[string][]string{}, nil
	}

	placeholders := make([]string, 0, len(messageIDs))
	args := make([]any, 0, len(messageIDs))
	for _, id := range messageIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	query := fmt.Sprintf("SELECT guid, reactions FROM fray_messages WHERE guid IN (%s)", strings.Join(placeholders, ", "))
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]map[string][]string)
	for rows.Next() {
		var guid string
		var reactionsJSON string
		if err := rows.Scan(&guid, &reactionsJSON); err != nil {
			return nil, err
		}
		var reactions map[string][]string
		if reactionsJSON != "" {
			if err := json.Unmarshal([]byte(reactionsJSON), &reactions); err != nil {
				return nil, err
			}
		}
		result[guid] = normalizeReactions(reactions)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// EditMessage updates message body.
func EditMessage(db *sql.DB, messageID, newBody, agentID string) error {
	msg, err := GetMessage(db, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("message %s not found", messageID)
	}
	if msg.FromAgent != agentID {
		return fmt.Errorf("cannot edit message from another agent (message from %s)", msg.FromAgent)
	}

	editedAt := time.Now().Unix()
	if _, err := db.Exec("UPDATE fray_messages SET body = ?, edited_at = ? WHERE guid = ?", newBody, editedAt, messageID); err != nil {
		return err
	}
	return nil
}

// DeleteMessage marks a message as deleted.
func DeleteMessage(db *sql.DB, messageID string) error {
	msg, err := GetMessage(db, messageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("message %s not found", messageID)
	}

	deletedAt := time.Now().Unix()
	_, err = db.Exec("UPDATE fray_messages SET body = ?, archived_at = ? WHERE guid = ?", "[deleted]", deletedAt, messageID)
	return err
}

// GetConfig returns a config value.
func GetConfig(db *sql.DB, key string) (string, error) {
	row := db.QueryRow("SELECT value FROM fray_config WHERE key = ?", key)
	var value string
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

// SetConfig sets a config value.
func SetConfig(db *sql.DB, key, value string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO fray_config (key, value) VALUES (?, ?)", key, value)
	return err
}

// GetAllConfig returns all config entries.
func GetAllConfig(db *sql.DB) ([]types.ConfigEntry, error) {
	rows, err := db.Query("SELECT key, value FROM fray_config ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []types.ConfigEntry
	for rows.Next() {
		var entry types.ConfigEntry
		if err := rows.Scan(&entry.Key, &entry.Value); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// GetLinkedProject returns a linked project by alias.
func GetLinkedProject(db *sql.DB, alias string) (*types.LinkedProject, error) {
	row := db.QueryRow("SELECT alias, path FROM fray_linked_projects WHERE alias = ?", alias)
	var project types.LinkedProject
	if err := row.Scan(&project.Alias, &project.Path); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &project, nil
}

// GetLinkedProjects returns all linked projects.
func GetLinkedProjects(db *sql.DB) ([]types.LinkedProject, error) {
	rows, err := db.Query("SELECT alias, path FROM fray_linked_projects ORDER BY alias")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []types.LinkedProject
	for rows.Next() {
		var project types.LinkedProject
		if err := rows.Scan(&project.Alias, &project.Path); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

// LinkProject creates or updates a project alias.
func LinkProject(db *sql.DB, alias, path string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO fray_linked_projects (alias, path) VALUES (?, ?)", alias, path)
	return err
}

// UnlinkProject removes a project alias.
func UnlinkProject(db *sql.DB, alias string) (bool, error) {
	result, err := db.Exec("DELETE FROM fray_linked_projects WHERE alias = ?", alias)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetFilter returns filter preferences for an agent.
func GetFilter(db *sql.DB, agentID string) (*types.Filter, error) {
	row := db.QueryRow("SELECT agent_id, mentions_pattern FROM fray_filters WHERE agent_id = ?", agentID)
	var filter types.Filter
	var mentions sql.NullString
	if err := row.Scan(&filter.AgentID, &mentions); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if mentions.Valid {
		filter.MentionsPattern = &mentions.String
	}
	return &filter, nil
}

// SetFilter upserts filter preferences.
func SetFilter(db *sql.DB, filter types.Filter) error {
	_, err := db.Exec(`
		INSERT INTO fray_filters (agent_id, mentions_pattern)
		VALUES (?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
		  mentions_pattern = excluded.mentions_pattern
	`, filter.AgentID, filter.MentionsPattern)
	return err
}

// ClearFilter clears filter preferences.
func ClearFilter(db *sql.DB, agentID string) error {
	_, err := db.Exec("DELETE FROM fray_filters WHERE agent_id = ?", agentID)
	return err
}

// MarkMessagesRead records read receipts.
func MarkMessagesRead(db *sql.DB, messageIDs []string, agentPrefix string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	now := time.Now().Unix()
	for _, id := range messageIDs {
		if _, err := db.Exec(`
			INSERT OR IGNORE INTO fray_read_receipts (message_guid, agent_prefix, read_at)
			VALUES (?, ?, ?)
		`, id, agentPrefix, now); err != nil {
			return err
		}
	}
	return nil
}

// GetReadReceipts returns agent prefixes who read a message.
func GetReadReceipts(db *sql.DB, messageID string) ([]string, error) {
	rows, err := db.Query(`
		SELECT agent_prefix FROM fray_read_receipts
		WHERE message_guid = ?
		ORDER BY read_at
	`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var receipts []string
	for rows.Next() {
		var prefix string
		if err := rows.Scan(&prefix); err != nil {
			return nil, err
		}
		receipts = append(receipts, prefix)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

// GetReadReceiptCount returns count of read receipts.
func GetReadReceiptCount(db *sql.DB, messageID string) (int64, error) {
	row := db.QueryRow(`
		SELECT COUNT(*) as count FROM fray_read_receipts WHERE message_guid = ?
	`, messageID)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ArchiveMessages archives messages before a cursor.
func ArchiveMessages(db *sql.DB, before *types.MessageCursor, beforeID string) (int64, error) {
	archivedAt := time.Now().Unix()

	if before == nil && beforeID == "" {
		result, err := db.Exec("UPDATE fray_messages SET archived_at = ? WHERE archived_at IS NULL", archivedAt)
		if err != nil {
			return 0, err
		}
		return result.RowsAffected()
	}

	cursor, err := resolveCursor(db, before, beforeID)
	if err != nil {
		return 0, err
	}
	if cursor == nil {
		return 0, fmt.Errorf("message not found: %s", beforeID)
	}

	clause, params := buildCursorCondition("", "<", cursor)
	query := fmt.Sprintf("UPDATE fray_messages SET archived_at = ? WHERE %s AND archived_at IS NULL", clause)
	params = append([]any{archivedAt}, params...)
	result, err := db.Exec(query, params...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetReplyChain returns parent + replies.
func GetReplyChain(db *sql.DB, messageID string) ([]types.Message, error) {
	rows, err := db.Query(`
		SELECT * FROM fray_messages
		WHERE guid = ? OR reply_to = ?
		ORDER BY CASE WHEN guid = ? THEN 0 ELSE 1 END, ts ASC, guid ASC
	`, messageID, messageID, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows)
}

// GetReplyCount returns the number of replies to a message.
func GetReplyCount(db *sql.DB, messageID string) (int64, error) {
	row := db.QueryRow("SELECT COUNT(*) as count FROM fray_messages WHERE reply_to = ?", messageID)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// QuestionUpdates represents partial question updates.
type QuestionUpdates struct {
	Status     types.OptionalString
	ToAgent    types.OptionalString
	ThreadGUID types.OptionalString
	AskedIn    types.OptionalString
	AnsweredIn types.OptionalString
}

// CreateQuestion inserts a new question.
func CreateQuestion(db *sql.DB, question types.Question) (types.Question, error) {
	guid := question.GUID
	if guid == "" {
		var err error
		guid, err = generateUniqueGUIDForTable(db, "fray_questions", "qstn")
		if err != nil {
			return types.Question{}, err
		}
	}

	status := question.Status
	if status == "" {
		status = types.QuestionStatusUnasked
	}
	createdAt := question.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	_, err := db.Exec(`
		INSERT INTO fray_questions (guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, guid, question.Re, question.FromAgent, question.ToAgent, string(status), question.ThreadGUID, question.AskedIn, question.AnsweredIn, createdAt)
	if err != nil {
		return types.Question{}, err
	}

	question.GUID = guid
	question.Status = status
	question.CreatedAt = createdAt
	return question, nil
}

// UpdateQuestion updates question fields.
func UpdateQuestion(db *sql.DB, guid string, updates QuestionUpdates) (*types.Question, error) {
	var fields []string
	var args []any

	if updates.Status.Set {
		fields = append(fields, "status = ?")
		args = append(args, nullableValue(updates.Status.Value))
	}
	if updates.ToAgent.Set {
		fields = append(fields, "to_agent = ?")
		args = append(args, nullableValue(updates.ToAgent.Value))
	}
	if updates.ThreadGUID.Set {
		fields = append(fields, "thread_guid = ?")
		args = append(args, nullableValue(updates.ThreadGUID.Value))
	}
	if updates.AskedIn.Set {
		fields = append(fields, "asked_in = ?")
		args = append(args, nullableValue(updates.AskedIn.Value))
	}
	if updates.AnsweredIn.Set {
		fields = append(fields, "answered_in = ?")
		args = append(args, nullableValue(updates.AnsweredIn.Value))
	}

	if len(fields) == 0 {
		return GetQuestion(db, guid)
	}

	args = append(args, guid)
	query := fmt.Sprintf("UPDATE fray_questions SET %s WHERE guid = ?", strings.Join(fields, ", "))
	if _, err := db.Exec(query, args...); err != nil {
		return nil, err
	}
	return GetQuestion(db, guid)
}

// GetQuestion returns a question by GUID.
func GetQuestion(db *sql.DB, guid string) (*types.Question, error) {
	row := db.QueryRow(`
		SELECT guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, created_at
		FROM fray_questions WHERE guid = ?
	`, guid)

	question, err := scanQuestion(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &question, nil
}

// GetQuestionByPrefix returns the first question matching a GUID prefix.
func GetQuestionByPrefix(db *sql.DB, prefix string) (*types.Question, error) {
	rows, err := db.Query(`
		SELECT guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, created_at
		FROM fray_questions
		WHERE guid = ? OR guid LIKE ?
		ORDER BY created_at DESC
	`, prefix, fmt.Sprintf("%s%%", prefix))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}
	question, err := scanQuestion(rows)
	if err != nil {
		return nil, err
	}
	return &question, nil
}

// GetQuestionsByRe returns questions matching the provided text.
func GetQuestionsByRe(db *sql.DB, re string) ([]types.Question, error) {
	rows, err := db.Query(`
		SELECT guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, created_at
		FROM fray_questions
		WHERE lower(re) = lower(?)
		ORDER BY created_at ASC
	`, strings.TrimSpace(re))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanQuestions(rows)
}

// GetQuestions returns questions filtered by options.
func GetQuestions(db *sql.DB, options *types.QuestionQueryOptions) ([]types.Question, error) {
	query := `
		SELECT guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, created_at
		FROM fray_questions
	`
	var conditions []string
	var args []any

	if options != nil {
		if len(options.Statuses) > 0 {
			placeholders := make([]string, 0, len(options.Statuses))
			for _, status := range options.Statuses {
				placeholders = append(placeholders, "?")
				args = append(args, string(status))
			}
			conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ", ")))
		}
		if options.ThreadGUID != nil {
			conditions = append(conditions, "thread_guid = ?")
			args = append(args, *options.ThreadGUID)
		} else if options.RoomOnly {
			conditions = append(conditions, "thread_guid IS NULL")
		}
		if options.ToAgent != nil && *options.ToAgent != "" {
			conditions = append(conditions, "to_agent = ?")
			args = append(args, *options.ToAgent)
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanQuestions(rows)
}

// ThreadUpdates represents partial thread updates.
type ThreadUpdates struct {
	Name         types.OptionalString
	Status       types.OptionalString
	ParentThread types.OptionalString
}

// CreateThread inserts a new thread.
func CreateThread(db *sql.DB, thread types.Thread) (types.Thread, error) {
	guid := thread.GUID
	if guid == "" {
		var err error
		guid, err = generateUniqueGUIDForTable(db, "fray_threads", "thrd")
		if err != nil {
			return types.Thread{}, err
		}
	}

	status := thread.Status
	if status == "" {
		status = types.ThreadStatusOpen
	}
	createdAt := thread.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	_, err := db.Exec(`
		INSERT INTO fray_threads (guid, name, parent_thread, status, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, guid, thread.Name, thread.ParentThread, string(status), createdAt)
	if err != nil {
		return types.Thread{}, err
	}

	thread.GUID = guid
	thread.Status = status
	thread.CreatedAt = createdAt
	return thread, nil
}

// UpdateThread updates thread fields.
func UpdateThread(db *sql.DB, guid string, updates ThreadUpdates) (*types.Thread, error) {
	var fields []string
	var args []any

	if updates.Name.Set {
		fields = append(fields, "name = ?")
		args = append(args, nullableValue(updates.Name.Value))
	}
	if updates.Status.Set {
		fields = append(fields, "status = ?")
		args = append(args, nullableValue(updates.Status.Value))
	}
	if updates.ParentThread.Set {
		fields = append(fields, "parent_thread = ?")
		args = append(args, nullableValue(updates.ParentThread.Value))
	}

	if len(fields) == 0 {
		return GetThread(db, guid)
	}

	args = append(args, guid)
	query := fmt.Sprintf("UPDATE fray_threads SET %s WHERE guid = ?", strings.Join(fields, ", "))
	if _, err := db.Exec(query, args...); err != nil {
		return nil, err
	}
	return GetThread(db, guid)
}

// GetThread returns a thread by GUID.
func GetThread(db *sql.DB, guid string) (*types.Thread, error) {
	row := db.QueryRow(`
		SELECT guid, name, parent_thread, status, created_at
		FROM fray_threads WHERE guid = ?
	`, guid)

	thread, err := scanThread(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &thread, nil
}

// GetThreadByPrefix returns the first thread matching a GUID prefix.
func GetThreadByPrefix(db *sql.DB, prefix string) (*types.Thread, error) {
	rows, err := db.Query(`
		SELECT guid, name, parent_thread, status, created_at
		FROM fray_threads
		WHERE guid = ? OR guid LIKE ?
		ORDER BY created_at ASC
	`, prefix, fmt.Sprintf("%s%%", prefix))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}
	thread, err := scanThread(rows)
	if err != nil {
		return nil, err
	}
	return &thread, nil
}

// GetThreadByName returns a thread by name and optional parent.
func GetThreadByName(db *sql.DB, name string, parent *string) (*types.Thread, error) {
	var row *sql.Row
	if parent == nil {
		row = db.QueryRow(`
			SELECT guid, name, parent_thread, status, created_at
			FROM fray_threads WHERE name = ? AND parent_thread IS NULL
		`, name)
	} else {
		row = db.QueryRow(`
			SELECT guid, name, parent_thread, status, created_at
			FROM fray_threads WHERE name = ? AND parent_thread = ?
		`, name, *parent)
	}

	thread, err := scanThread(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &thread, nil
}

// GetThreads returns threads filtered by options.
func GetThreads(db *sql.DB, options *types.ThreadQueryOptions) ([]types.Thread, error) {
	query := `
		SELECT DISTINCT t.guid, t.name, t.parent_thread, t.status, t.created_at
		FROM fray_threads t
	`
	var conditions []string
	var args []any

	if options != nil && options.SubscribedAgent != nil {
		query += " INNER JOIN fray_thread_subscriptions s ON s.thread_guid = t.guid"
		conditions = append(conditions, "s.agent_id = ?")
		args = append(args, *options.SubscribedAgent)
	}

	if options != nil {
		if options.ParentThread != nil {
			conditions = append(conditions, "t.parent_thread = ?")
			args = append(args, *options.ParentThread)
		}
		if options.Status != nil {
			conditions = append(conditions, "t.status = ?")
			args = append(args, string(*options.Status))
		} else if !options.IncludeArchived {
			conditions = append(conditions, "t.status = ?")
			args = append(args, string(types.ThreadStatusOpen))
		}
	} else {
		conditions = append(conditions, "t.status = ?")
		args = append(args, string(types.ThreadStatusOpen))
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY t.created_at ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanThreads(rows)
}

// SubscribeThread subscribes an agent to a thread.
func SubscribeThread(db *sql.DB, threadGUID, agentID string, subscribedAt int64) error {
	if subscribedAt == 0 {
		subscribedAt = time.Now().Unix()
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_thread_subscriptions (thread_guid, agent_id, subscribed_at)
		VALUES (?, ?, ?)
	`, threadGUID, agentID, subscribedAt)
	return err
}

// UnsubscribeThread unsubscribes an agent from a thread.
func UnsubscribeThread(db *sql.DB, threadGUID, agentID string) error {
	_, err := db.Exec(`
		DELETE FROM fray_thread_subscriptions WHERE thread_guid = ? AND agent_id = ?
	`, threadGUID, agentID)
	return err
}

// AddMessageToThread adds a message to a thread playlist.
func AddMessageToThread(db *sql.DB, threadGUID, messageGUID, addedBy string, addedAt int64) error {
	if addedAt == 0 {
		addedAt = time.Now().Unix()
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_thread_messages (thread_guid, message_guid, added_by, added_at)
		VALUES (?, ?, ?, ?)
	`, threadGUID, messageGUID, addedBy, addedAt)
	return err
}

// RemoveMessageFromThread removes a message from a thread playlist.
func RemoveMessageFromThread(db *sql.DB, threadGUID, messageGUID string) error {
	_, err := db.Exec(`
		DELETE FROM fray_thread_messages WHERE thread_guid = ? AND message_guid = ?
	`, threadGUID, messageGUID)
	return err
}

// GetThreadMessages returns messages in a thread (home or membership).
func GetThreadMessages(db *sql.DB, threadGUID string) ([]types.Message, error) {
	rows, err := db.Query(`
		SELECT DISTINCT m.* FROM fray_messages m
		LEFT JOIN fray_thread_messages tm ON tm.message_guid = m.guid AND tm.thread_guid = ?
		WHERE m.home = ? OR tm.thread_guid = ?
		ORDER BY m.ts ASC, m.guid ASC
	`, threadGUID, threadGUID, threadGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows)
}

// IsMessageInThread reports whether a message is in a thread (home or membership).
func IsMessageInThread(db *sql.DB, threadGUID, messageGUID string) (bool, error) {
	row := db.QueryRow(`
		SELECT 1 FROM fray_messages WHERE guid = ? AND home = ?
		UNION
		SELECT 1 FROM fray_thread_messages WHERE message_guid = ? AND thread_guid = ?
		LIMIT 1
	`, messageGUID, threadGUID, messageGUID, threadGUID)
	var value int
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// PruneExpiredClaims removes expired claims.
func PruneExpiredClaims(db *sql.DB) (int64, error) {
	now := time.Now().Unix()
	result, err := db.Exec("DELETE FROM fray_claims WHERE expires_at IS NOT NULL AND expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CreateClaim inserts a new claim.
func CreateClaim(db *sql.DB, claim types.ClaimInput) (*types.Claim, error) {
	now := time.Now().Unix()
	result, err := db.Exec(`
		INSERT INTO fray_claims (agent_id, claim_type, pattern, reason, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, claim.AgentID, claim.ClaimType, claim.Pattern, claim.Reason, now, claim.ExpiresAt)
	if err != nil {
		if isConstraintError(err) {
			existing, lookupErr := GetClaim(db, claim.ClaimType, claim.Pattern)
			if lookupErr == nil && existing != nil {
				return nil, fmt.Errorf("already claimed by @%s: %s:%s", existing.AgentID, claim.ClaimType, claim.Pattern)
			}
		}
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &types.Claim{
		ID:        id,
		AgentID:   claim.AgentID,
		ClaimType: claim.ClaimType,
		Pattern:   claim.Pattern,
		Reason:    claim.Reason,
		CreatedAt: now,
		ExpiresAt: claim.ExpiresAt,
	}, nil
}

// GetClaim returns a claim by type and pattern.
func GetClaim(db *sql.DB, claimType types.ClaimType, pattern string) (*types.Claim, error) {
	row := db.QueryRow(`
		SELECT id, agent_id, claim_type, pattern, reason, created_at, expires_at
		FROM fray_claims
		WHERE claim_type = ? AND pattern = ?
	`, claimType, pattern)
	claim, err := scanClaim(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &claim, nil
}

// GetClaimsByAgent returns claims for an agent.
func GetClaimsByAgent(db *sql.DB, agentID string) ([]types.Claim, error) {
	rows, err := db.Query(`
		SELECT id, agent_id, claim_type, pattern, reason, created_at, expires_at
		FROM fray_claims
		WHERE agent_id = ?
		ORDER BY created_at
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanClaims(rows)
}

// GetClaimsByType returns claims of a type.
func GetClaimsByType(db *sql.DB, claimType types.ClaimType) ([]types.Claim, error) {
	rows, err := db.Query(`
		SELECT id, agent_id, claim_type, pattern, reason, created_at, expires_at
		FROM fray_claims
		WHERE claim_type = ?
		ORDER BY created_at
	`, claimType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanClaims(rows)
}

// GetAllClaims returns all active claims.
func GetAllClaims(db *sql.DB) ([]types.Claim, error) {
	if _, err := PruneExpiredClaims(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT id, agent_id, claim_type, pattern, reason, created_at, expires_at
		FROM fray_claims
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanClaims(rows)
}

// DeleteClaim removes a specific claim.
func DeleteClaim(db *sql.DB, claimType types.ClaimType, pattern string) (bool, error) {
	result, err := db.Exec("DELETE FROM fray_claims WHERE claim_type = ? AND pattern = ?", claimType, pattern)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteClaimsByAgent removes all claims for an agent.
func DeleteClaimsByAgent(db *sql.DB, agentID string) (int64, error) {
	result, err := db.Exec("DELETE FROM fray_claims WHERE agent_id = ?", agentID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// FindConflictingFileClaims returns conflicting file claims.
func FindConflictingFileClaims(db *sql.DB, filePaths []string, excludeAgent string) ([]types.Claim, error) {
	claims, err := GetClaimsByType(db, types.ClaimTypeFile)
	if err != nil {
		return nil, err
	}

	var conflicts []types.Claim
	for _, claim := range claims {
		if excludeAgent != "" && claim.AgentID == excludeAgent {
			continue
		}
		matcher, err := glob.Compile(claim.Pattern)
		if err != nil {
			continue
		}
		for _, filePath := range filePaths {
			if matcher.Match(filePath) {
				conflicts = append(conflicts, claim)
				break
			}
		}
	}

	return conflicts, nil
}

// CheckFileConflict returns the first conflicting file claim.
func CheckFileConflict(db *sql.DB, filePath, excludeAgent string) (*types.Claim, error) {
	conflicts, err := FindConflictingFileClaims(db, []string{filePath}, excludeAgent)
	if err != nil {
		return nil, err
	}
	if len(conflicts) == 0 {
		return nil, nil
	}
	return &conflicts[0], nil
}

// UpdateClaimsAgentID updates claim ownership.
func UpdateClaimsAgentID(db *sql.DB, oldID, newID string) error {
	_, err := db.Exec("UPDATE fray_claims SET agent_id = ? WHERE agent_id = ?", newID, oldID)
	return err
}

// GetClaimCountsByAgent returns counts grouped by agent.
func GetClaimCountsByAgent(db *sql.DB) (map[string]int64, error) {
	if _, err := PruneExpiredClaims(db); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT agent_id, COUNT(*) as count
		FROM fray_claims
		GROUP BY agent_id
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

func resolveCursor(db *sql.DB, cursor *types.MessageCursor, guid string) (*types.MessageCursor, error) {
	if cursor != nil {
		return cursor, nil
	}
	if guid == "" {
		return nil, nil
	}
	row := db.QueryRow("SELECT guid, ts FROM fray_messages WHERE guid = ?", guid)
	var result types.MessageCursor
	if err := row.Scan(&result.GUID, &result.TS); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func buildCursorCondition(prefix, op string, cursor *types.MessageCursor) (string, []any) {
	tsCol := fmt.Sprintf("%sts", prefix)
	guidCol := fmt.Sprintf("%sguid", prefix)
	clause := fmt.Sprintf("(%s %s ? OR (%s = ? AND %s %s ?))", tsCol, op, tsCol, guidCol, op)
	return clause, []any{cursor.TS, cursor.TS, cursor.GUID}
}

func buildFilterCondition(filter *types.Filter) (string, []any) {
	if filter == nil || filter.MentionsPattern == nil || *filter.MentionsPattern == "" {
		return "", nil
	}
	return "EXISTS (SELECT 1 FROM json_each(mentions) WHERE value LIKE ?)", []any{*filter.MentionsPattern}
}

func scanMessages(rows *sql.Rows) ([]types.Message, error) {
	var messages []types.Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func scanMessage(scanner interface{ Scan(dest ...any) error }) (types.Message, error) {
	var row messageRow
	if err := scanner.Scan(&row.GUID, &row.TS, &row.ChannelID, &row.Home, &row.FromAgent, &row.Body, &row.Mentions, &row.MsgType, &row.References, &row.SurfaceMessage, &row.ReplyTo, &row.EditedAt, &row.ArchivedAt, &row.Reactions); err != nil {
		return types.Message{}, err
	}
	return row.toMessage()
}

func scanQuestion(scanner interface{ Scan(dest ...any) error }) (types.Question, error) {
	var row questionRow
	if err := scanner.Scan(&row.GUID, &row.Re, &row.FromAgent, &row.ToAgent, &row.Status, &row.ThreadGUID, &row.AskedIn, &row.AnsweredIn, &row.CreatedAt); err != nil {
		return types.Question{}, err
	}
	return row.toQuestion(), nil
}

func scanQuestions(rows *sql.Rows) ([]types.Question, error) {
	var questions []types.Question
	for rows.Next() {
		question, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		questions = append(questions, question)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return questions, nil
}

func scanThread(scanner interface{ Scan(dest ...any) error }) (types.Thread, error) {
	var row threadRow
	if err := scanner.Scan(&row.GUID, &row.Name, &row.ParentThread, &row.Status, &row.CreatedAt); err != nil {
		return types.Thread{}, err
	}
	return row.toThread(), nil
}

func scanThreads(rows *sql.Rows) ([]types.Thread, error) {
	var threads []types.Thread
	for rows.Next() {
		thread, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return threads, nil
}

func scanAgent(scanner interface{ Scan(dest ...any) error }) (types.Agent, error) {
	var row agentRow
	if err := scanner.Scan(&row.GUID, &row.AgentID, &row.Status, &row.Purpose, &row.RegisteredAt, &row.LastSeen, &row.LeftAt); err != nil {
		return types.Agent{}, err
	}
	return row.toAgent(), nil
}

func scanClaim(scanner interface{ Scan(dest ...any) error }) (types.Claim, error) {
	var row claimRow
	if err := scanner.Scan(&row.ID, &row.AgentID, &row.ClaimType, &row.Pattern, &row.Reason, &row.CreatedAt, &row.ExpiresAt); err != nil {
		return types.Claim{}, err
	}
	return row.toClaim(), nil
}

func scanClaims(rows *sql.Rows) ([]types.Claim, error) {
	var claims []types.Claim
	for rows.Next() {
		claim, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return claims, nil
}

func generateUniqueGUIDForTable(db *sql.DB, table, prefix string) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		guid, err := core.GenerateGUID(prefix)
		if err != nil {
			return "", err
		}
		row := db.QueryRow(fmt.Sprintf("SELECT 1 FROM %s WHERE guid = ?", table), guid)
		var exists int
		err = row.Scan(&exists)
		if err == sql.ErrNoRows {
			return guid, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to generate unique %s GUID", prefix)
}

func isNumericSuffix(s string) bool {
	return parseNumeric(s) > 0 && fmt.Sprintf("%d", parseNumeric(s)) == s
}

func parseNumeric(s string) int {
	value := 0
	if s == "" {
		return 0
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		value = value*10 + int(r-'0')
	}
	return value
}

func nullableValue[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}

func isConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		return code == sqliteConstraint || code == sqliteConstraintUnique
	}
	return false
}

type messageRow struct {
	GUID           string
	TS             int64
	ChannelID      sql.NullString
	Home           sql.NullString
	FromAgent      string
	Body           string
	Mentions       string
	Reactions      string
	MsgType        sql.NullString
	References     sql.NullString
	SurfaceMessage sql.NullString
	ReplyTo        sql.NullString
	EditedAt       sql.NullInt64
	ArchivedAt     sql.NullInt64
}

type questionRow struct {
	GUID       string
	Re         string
	FromAgent  string
	ToAgent    sql.NullString
	Status     sql.NullString
	ThreadGUID sql.NullString
	AskedIn    sql.NullString
	AnsweredIn sql.NullString
	CreatedAt  int64
}

func (row questionRow) toQuestion() types.Question {
	status := types.QuestionStatusUnasked
	if row.Status.Valid && row.Status.String != "" {
		status = types.QuestionStatus(row.Status.String)
	}
	return types.Question{
		GUID:       row.GUID,
		Re:         row.Re,
		FromAgent:  row.FromAgent,
		ToAgent:    nullStringPtr(row.ToAgent),
		Status:     status,
		ThreadGUID: nullStringPtr(row.ThreadGUID),
		AskedIn:    nullStringPtr(row.AskedIn),
		AnsweredIn: nullStringPtr(row.AnsweredIn),
		CreatedAt:  row.CreatedAt,
	}
}

type threadRow struct {
	GUID         string
	Name         string
	ParentThread sql.NullString
	Status       sql.NullString
	CreatedAt    int64
}

func (row threadRow) toThread() types.Thread {
	status := types.ThreadStatusOpen
	if row.Status.Valid && row.Status.String != "" {
		status = types.ThreadStatus(row.Status.String)
	}
	return types.Thread{
		GUID:         row.GUID,
		Name:         row.Name,
		ParentThread: nullStringPtr(row.ParentThread),
		Status:       status,
		CreatedAt:    row.CreatedAt,
	}
}

func (row messageRow) toMessage() (types.Message, error) {
	mentions := []string{}
	if row.Mentions != "" {
		if err := json.Unmarshal([]byte(row.Mentions), &mentions); err != nil {
			return types.Message{}, err
		}
	}
	reactions := map[string][]string{}
	if row.Reactions != "" {
		if err := json.Unmarshal([]byte(row.Reactions), &reactions); err != nil {
			return types.Message{}, err
		}
	}
	msgType := types.MessageTypeAgent
	if row.MsgType.Valid && row.MsgType.String != "" {
		msgType = types.MessageType(row.MsgType.String)
	}
	home := "room"
	if row.Home.Valid && row.Home.String != "" {
		home = row.Home.String
	}

	return types.Message{
		ID:             row.GUID,
		TS:             row.TS,
		ChannelID:      nullStringPtr(row.ChannelID),
		Home:           home,
		FromAgent:      row.FromAgent,
		Body:           row.Body,
		Mentions:       mentions,
		Reactions:      normalizeReactions(reactions),
		Type:           msgType,
		References:     nullStringPtr(row.References),
		SurfaceMessage: nullStringPtr(row.SurfaceMessage),
		ReplyTo:        nullStringPtr(row.ReplyTo),
		EditedAt:       nullIntPtr(row.EditedAt),
		ArchivedAt:     nullIntPtr(row.ArchivedAt),
	}, nil
}

type agentRow struct {
	GUID         string
	AgentID      string
	Status       sql.NullString
	Purpose      sql.NullString
	RegisteredAt int64
	LastSeen     int64
	LeftAt       sql.NullInt64
}

func (row agentRow) toAgent() types.Agent {
	return types.Agent{
		GUID:         row.GUID,
		AgentID:      row.AgentID,
		Status:       nullStringPtr(row.Status),
		Purpose:      nullStringPtr(row.Purpose),
		RegisteredAt: row.RegisteredAt,
		LastSeen:     row.LastSeen,
		LeftAt:       nullIntPtr(row.LeftAt),
	}
}

type claimRow struct {
	ID        int64
	AgentID   string
	ClaimType types.ClaimType
	Pattern   string
	Reason    sql.NullString
	CreatedAt int64
	ExpiresAt sql.NullInt64
}

func (row claimRow) toClaim() types.Claim {
	return types.Claim{
		ID:        row.ID,
		AgentID:   row.AgentID,
		ClaimType: row.ClaimType,
		Pattern:   row.Pattern,
		Reason:    nullStringPtr(row.Reason),
		CreatedAt: row.CreatedAt,
		ExpiresAt: nullIntPtr(row.ExpiresAt),
	}
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func nullIntPtr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}
