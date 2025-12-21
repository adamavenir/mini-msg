package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/types"
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
		FROM mm_agents
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
		FROM mm_agents
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

// CreateAgent inserts a new agent.
func CreateAgent(db *sql.DB, agent types.Agent) error {
	guid := agent.GUID
	if guid == "" {
		var err error
		guid, err = generateUniqueGUIDForTable(db, "mm_agents", "usr")
		if err != nil {
			return err
		}
	}

	_, err := db.Exec(`
		INSERT INTO mm_agents (guid, agent_id, status, purpose, registered_at, last_seen, left_at)
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
	query := fmt.Sprintf("UPDATE mm_agents SET %s WHERE agent_id = ?", strings.Join(fields, ", "))
	_, err := db.Exec(query, args...)
	return err
}

// GetActiveAgents returns non-stale agents.
func GetActiveAgents(db *sql.DB, staleHours int) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, registered_at, last_seen, left_at
		FROM mm_agents
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
		FROM mm_agents
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
	rows, err := db.Query(`SELECT agent_id FROM mm_agents`)
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
		SELECT 1 FROM mm_agents
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

	if _, err := db.Exec("UPDATE mm_agents SET agent_id = ? WHERE agent_id = ?", newID, oldID); err != nil {
		return err
	}
	if _, err := db.Exec("UPDATE mm_messages SET from_agent = ? WHERE from_agent = ?", newID, oldID); err != nil {
		return err
	}

	rows, err := db.Query(`
		SELECT guid, mentions FROM mm_messages
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
		if _, err := db.Exec("UPDATE mm_messages SET mentions = ? WHERE guid = ?", string(updatedJSON), guid); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return UpdateClaimsAgentID(db, oldID, newID)
}

// GetMaxVersion returns the highest version for a base name.
func GetMaxVersion(db *sql.DB, base string) (int, error) {
	pattern := fmt.Sprintf("%s.[0-9]*", base)
	rows, err := db.Query("SELECT agent_id FROM mm_agents WHERE agent_id GLOB ?", pattern)
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

	msgType := message.Type
	if msgType == "" {
		msgType = types.MessageTypeAgent
	}

	guid, err := generateUniqueGUIDForTable(db, "mm_messages", "msg")
	if err != nil {
		return types.Message{}, err
	}

	_, err = db.Exec(`
		INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)
	`, guid, ts, channelID, message.FromAgent, message.Body, string(mentionsJSON), msgType, message.ReplyTo)
	if err != nil {
		return types.Message{}, err
	}

	return types.Message{
		ID:         guid,
		TS:         ts,
		ChannelID:  channelID,
		FromAgent:  message.FromAgent,
		Body:       message.Body,
		Mentions:   message.Mentions,
		Type:       msgType,
		ReplyTo:    message.ReplyTo,
		EditedAt:   nil,
		ArchivedAt: nil,
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
	if options != nil {
		limit = options.Limit
		includeArchived = options.IncludeArchived
		filter = options.Filter
	}

	if limit > 0 && sinceCursor == nil && beforeCursor == nil {
		var conditions []string
		var params []any

		if !includeArchived {
			conditions = append(conditions, "archived_at IS NULL")
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
				SELECT * FROM mm_messages%s
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

	query := "SELECT * FROM mm_messages"
	var conditions []string
	var params []any

	if !includeArchived {
		conditions = append(conditions, "archived_at IS NULL")
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
	if options != nil {
		filterUnread = options.UnreadOnly
		if options.AgentPrefix != "" {
			agentPrefix = options.AgentPrefix
		}
		includeArchived = options.IncludeArchived
		limit = options.Limit
	}

	query := `
		SELECT DISTINCT m.* FROM mm_messages m, json_each(m.mentions) j
	`
	var params []any

	if filterUnread {
		query += `
		LEFT JOIN mm_read_receipts r
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
		SELECT guid, ts FROM mm_messages
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
	row := db.QueryRow("SELECT * FROM mm_messages WHERE guid = ?", messageID)
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
		SELECT * FROM mm_messages
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

// EditMessage updates message body and logs event.
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
	if _, err := db.Exec("UPDATE mm_messages SET body = ?, edited_at = ? WHERE guid = ?", newBody, editedAt, messageID); err != nil {
		return err
	}

	_, err = CreateMessage(db, types.Message{
		FromAgent: "system",
		Body:      fmt.Sprintf("update: _@%s edited message #%s_", agentID, messageID),
		Mentions:  []string{agentID},
		Type:      types.MessageTypeEvent,
	})
	return err
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
	_, err = db.Exec("UPDATE mm_messages SET body = ?, archived_at = ? WHERE guid = ?", "[deleted]", deletedAt, messageID)
	return err
}

// GetConfig returns a config value.
func GetConfig(db *sql.DB, key string) (string, error) {
	row := db.QueryRow("SELECT value FROM mm_config WHERE key = ?", key)
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
	_, err := db.Exec("INSERT OR REPLACE INTO mm_config (key, value) VALUES (?, ?)", key, value)
	return err
}

// GetAllConfig returns all config entries.
func GetAllConfig(db *sql.DB) ([]types.ConfigEntry, error) {
	rows, err := db.Query("SELECT key, value FROM mm_config ORDER BY key")
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
	row := db.QueryRow("SELECT alias, path FROM mm_linked_projects WHERE alias = ?", alias)
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
	rows, err := db.Query("SELECT alias, path FROM mm_linked_projects ORDER BY alias")
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
	_, err := db.Exec("INSERT OR REPLACE INTO mm_linked_projects (alias, path) VALUES (?, ?)", alias, path)
	return err
}

// UnlinkProject removes a project alias.
func UnlinkProject(db *sql.DB, alias string) (bool, error) {
	result, err := db.Exec("DELETE FROM mm_linked_projects WHERE alias = ?", alias)
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
	row := db.QueryRow("SELECT agent_id, mentions_pattern FROM mm_filters WHERE agent_id = ?", agentID)
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
		INSERT INTO mm_filters (agent_id, mentions_pattern)
		VALUES (?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
		  mentions_pattern = excluded.mentions_pattern
	`, filter.AgentID, filter.MentionsPattern)
	return err
}

// ClearFilter clears filter preferences.
func ClearFilter(db *sql.DB, agentID string) error {
	_, err := db.Exec("DELETE FROM mm_filters WHERE agent_id = ?", agentID)
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
			INSERT OR IGNORE INTO mm_read_receipts (message_guid, agent_prefix, read_at)
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
		SELECT agent_prefix FROM mm_read_receipts
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
		SELECT COUNT(*) as count FROM mm_read_receipts WHERE message_guid = ?
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
		result, err := db.Exec("UPDATE mm_messages SET archived_at = ? WHERE archived_at IS NULL", archivedAt)
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
	query := fmt.Sprintf("UPDATE mm_messages SET archived_at = ? WHERE %s AND archived_at IS NULL", clause)
	params = append([]any{archivedAt}, params...)
	result, err := db.Exec(query, params...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetThread returns parent + replies.
func GetThread(db *sql.DB, messageID string) ([]types.Message, error) {
	rows, err := db.Query(`
		SELECT * FROM mm_messages
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
	row := db.QueryRow("SELECT COUNT(*) as count FROM mm_messages WHERE reply_to = ?", messageID)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// PruneExpiredClaims removes expired claims.
func PruneExpiredClaims(db *sql.DB) (int64, error) {
	now := time.Now().Unix()
	result, err := db.Exec("DELETE FROM mm_claims WHERE expires_at IS NOT NULL AND expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CreateClaim inserts a new claim.
func CreateClaim(db *sql.DB, claim types.ClaimInput) (*types.Claim, error) {
	now := time.Now().Unix()
	result, err := db.Exec(`
		INSERT INTO mm_claims (agent_id, claim_type, pattern, reason, created_at, expires_at)
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
		FROM mm_claims
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
		FROM mm_claims
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
		FROM mm_claims
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
		FROM mm_claims
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
	result, err := db.Exec("DELETE FROM mm_claims WHERE claim_type = ? AND pattern = ?", claimType, pattern)
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
	result, err := db.Exec("DELETE FROM mm_claims WHERE agent_id = ?", agentID)
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
	_, err := db.Exec("UPDATE mm_claims SET agent_id = ? WHERE agent_id = ?", newID, oldID)
	return err
}

// GetClaimCountsByAgent returns counts grouped by agent.
func GetClaimCountsByAgent(db *sql.DB) (map[string]int64, error) {
	if _, err := PruneExpiredClaims(db); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT agent_id, COUNT(*) as count
		FROM mm_claims
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
	row := db.QueryRow("SELECT guid, ts FROM mm_messages WHERE guid = ?", guid)
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
	if err := scanner.Scan(&row.GUID, &row.TS, &row.ChannelID, &row.FromAgent, &row.Body, &row.Mentions, &row.MsgType, &row.ReplyTo, &row.EditedAt, &row.ArchivedAt); err != nil {
		return types.Message{}, err
	}
	return row.toMessage()
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
	GUID       string
	TS         int64
	ChannelID  sql.NullString
	FromAgent  string
	Body       string
	Mentions   string
	MsgType    sql.NullString
	ReplyTo    sql.NullString
	EditedAt   sql.NullInt64
	ArchivedAt sql.NullInt64
}

func (row messageRow) toMessage() (types.Message, error) {
	mentions := []string{}
	if row.Mentions != "" {
		if err := json.Unmarshal([]byte(row.Mentions), &mentions); err != nil {
			return types.Message{}, err
		}
	}
	msgType := types.MessageTypeAgent
	if row.MsgType.Valid && row.MsgType.String != "" {
		msgType = types.MessageType(row.MsgType.String)
	}

	return types.Message{
		ID:         row.GUID,
		TS:         row.TS,
		ChannelID:  nullStringPtr(row.ChannelID),
		FromAgent:  row.FromAgent,
		Body:       row.Body,
		Mentions:   mentions,
		Type:       msgType,
		ReplyTo:    nullStringPtr(row.ReplyTo),
		EditedAt:   nullIntPtr(row.EditedAt),
		ArchivedAt: nullIntPtr(row.ArchivedAt),
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
