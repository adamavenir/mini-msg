package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// messageColumns is the explicit column list for SELECT queries.
// This prevents column order issues when migrations add columns via ALTER TABLE.
const messageColumns = `guid, ts, channel_id, home, from_agent, body, mentions, type, "references", surface_message, reply_to, quote_message_guid, edited_at, archived_at, reactions`

// messageColumnsAliased is the same but with m. prefix for JOINs.
const messageColumnsAliased = `m.guid, m.ts, m.channel_id, m.home, m.from_agent, m.body, m.mentions, m.type, m."references", m.surface_message, m.reply_to, m.quote_message_guid, m.edited_at, m.archived_at, m.reactions`

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

	// New messages don't have reactions yet. Write empty JSON for legacy column.
	reactionsJSON := []byte("{}")

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
		INSERT INTO fray_messages (guid, ts, channel_id, home, from_agent, body, mentions, type, "references", surface_message, reply_to, quote_message_guid, edited_at, archived_at, reactions)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)
	`, guid, ts, channelID, home, message.FromAgent, message.Body, string(mentionsJSON), msgType, message.References, message.SurfaceMessage, message.ReplyTo, message.QuoteMessageGUID, string(reactionsJSON))
	if err != nil {
		return types.Message{}, err
	}

	// Return with empty reactions map (new messages don't have reactions)
	reactions := make(map[string][]types.ReactionEntry)

	return types.Message{
		ID:               guid,
		TS:               ts,
		ChannelID:        channelID,
		Home:             home,
		FromAgent:        message.FromAgent,
		Body:             message.Body,
		Mentions:         message.Mentions,
		Reactions:        reactions,
		Type:             msgType,
		References:       message.References,
		SurfaceMessage:   message.SurfaceMessage,
		ReplyTo:          message.ReplyTo,
		QuoteMessageGUID: message.QuoteMessageGUID,
		EditedAt:         nil,
		ArchivedAt:       nil,
	}, nil
}

// loadReactionsForMessages loads reactions from fray_reactions table into the messages.
func loadReactionsForMessages(db *sql.DB, messages []types.Message) error {
	if len(messages) == 0 {
		return nil
	}
	ids := make([]string, len(messages))
	for i, msg := range messages {
		ids[i] = msg.ID
	}
	reactionsMap, err := GetReactionsForMessages(db, ids)
	if err != nil {
		return err
	}
	for i := range messages {
		if reactions, ok := reactionsMap[messages[i].ID]; ok {
			messages[i].Reactions = reactions
		}
	}
	return nil
}

// scanMessagesWithReactions scans messages from rows and loads their reactions.
func scanMessagesWithReactions(db *sql.DB, rows *sql.Rows) ([]types.Message, error) {
	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if err := loadReactionsForMessages(db, messages); err != nil {
		return nil, err
	}
	return messages, nil
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
			SELECT %s FROM (
				SELECT %s FROM fray_messages%s
				ORDER BY ts DESC, guid DESC
				LIMIT ?
			) ORDER BY ts ASC, guid ASC
		`, messageColumns, messageColumns, whereClause)
		params = append(params, limit)

		rows, err := db.Query(query, params...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		return scanMessagesWithReactions(db, rows)
	}

	query := "SELECT " + messageColumns + " FROM fray_messages"
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

	return scanMessagesWithReactions(db, rows)
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
	includeReplies := ""
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
		includeReplies = options.IncludeRepliesToAgent
	}

	// Build common conditions
	var commonConditions []string
	var commonParams []any
	if !includeArchived {
		commonConditions = append(commonConditions, "m.archived_at IS NULL")
	}
	if home != "" {
		commonConditions = append(commonConditions, "m.home = ?")
		commonParams = append(commonParams, home)
	}
	if sinceCursor != nil {
		clause, args := buildCursorCondition("m.", ">", sinceCursor)
		commonConditions = append(commonConditions, clause)
		commonParams = append(commonParams, args...)
	}
	if beforeCursor != nil {
		clause, args := buildCursorCondition("m.", "<", beforeCursor)
		commonConditions = append(commonConditions, clause)
		commonParams = append(commonParams, args...)
	}

	conditionStr := ""
	if len(commonConditions) > 0 {
		conditionStr = " AND " + strings.Join(commonConditions, " AND ")
	}

	var query string
	var params []any

	if includeReplies != "" {
		// Use EXISTS for mentions + OR for replies to avoid cross-join issues
		query = `
		SELECT ` + messageColumnsAliased + ` FROM fray_messages m
		`
		if filterUnread {
			query += `
			LEFT JOIN fray_read_receipts r
			  ON m.guid = r.message_guid AND r.agent_prefix = ?
			`
			params = append(params, agentPrefix)
		}
		query += `
		WHERE (
			EXISTS (
				SELECT 1 FROM json_each(m.mentions) j
				WHERE j.value = 'all' OR j.value = ? OR j.value LIKE ?
			)
			OR m.reply_to IN (
				SELECT guid FROM fray_messages
				WHERE from_agent = ? OR from_agent LIKE ?
			)
		)
		`
		params = append(params, mentionPrefix, fmt.Sprintf("%s.%%", mentionPrefix))
		params = append(params, includeReplies, fmt.Sprintf("%s.%%", includeReplies))

		if filterUnread {
			query += " AND r.message_guid IS NULL"
		}
		query += conditionStr
		params = append(params, commonParams...)
	} else {
		// Original query for mentions only
		query = `
		SELECT DISTINCT ` + messageColumnsAliased + ` FROM fray_messages m, json_each(m.mentions) j
		`
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

		if filterUnread {
			query += " AND r.message_guid IS NULL"
		}
		query += conditionStr
		params = append(params, commonParams...)
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

	return scanMessagesWithReactions(db, rows)
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

// GetAgentLastPostTime returns the timestamp of the agent's most recent post.
// Returns 0 if the agent has never posted.
func GetAgentLastPostTime(database *sql.DB, agentID string) (int64, error) {
	var ts int64
	err := database.QueryRow(`
		SELECT ts FROM fray_messages
		WHERE from_agent = ?
		ORDER BY ts DESC
		LIMIT 1
	`, agentID).Scan(&ts)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return ts, nil
}

// GetMessage returns a message by GUID.
func GetMessage(db *sql.DB, messageID string) (*types.Message, error) {
	row := db.QueryRow("SELECT "+messageColumns+" FROM fray_messages WHERE guid = ?", messageID)
	message, err := scanMessage(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Load reactions from the new fray_reactions table
	reactions, err := GetReactionsForMessage(db, messageID)
	if err != nil {
		return nil, err
	}
	message.Reactions = reactions
	return &message, nil
}

// GetMessageByPrefix returns a message by GUID prefix.
func GetMessageByPrefix(db *sql.DB, prefix string) (*types.Message, error) {
	normalized := prefix
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(lower, "msg-") {
		normalized = normalized[4:]
	}

	rows, err := db.Query(fmt.Sprintf(`
		SELECT %s FROM fray_messages
		WHERE guid LIKE ?
		ORDER BY ts DESC
		LIMIT 2
	`, messageColumns), fmt.Sprintf("msg-%s%%", normalized))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(messages) == 1 {
		// Load reactions from the new fray_reactions table
		reactions, err := GetReactionsForMessage(db, messages[0].ID)
		if err != nil {
			return nil, err
		}
		messages[0].Reactions = reactions
		return &messages[0], nil
	}
	return nil, nil
}

// AddReaction adds a reaction for a message.
// Unlike the old implementation, reactions are no longer deduplicated - the same agent
// can react multiple times (each session counts). Returns the timestamp of the reaction.
func AddReaction(db *sql.DB, messageID, reactor, reaction string) (*types.Message, int64, error) {
	msg, err := GetMessage(db, messageID)
	if err != nil {
		return nil, 0, err
	}
	if msg == nil {
		return nil, 0, fmt.Errorf("message %s not found", messageID)
	}

	// Insert into new reactions table (no deduplication)
	reactedAt, err := InsertReactionNow(db, messageID, reactor, reaction)
	if err != nil {
		return nil, 0, err
	}

	// Load updated reactions from table
	reactions, err := GetReactionsForMessage(db, messageID)
	if err != nil {
		return nil, 0, err
	}
	msg.Reactions = reactions

	return msg, reactedAt, nil
}

// GetMessageReactionsNew returns reactions for the given message IDs from the fray_reactions table.
// This uses the new reactions format with timestamps.
func GetMessageReactionsNew(db *sql.DB, messageIDs []string) (map[string]map[string][]types.ReactionEntry, error) {
	return GetReactionsForMessages(db, messageIDs)
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
	rows, err := db.Query(fmt.Sprintf(`
		SELECT %s FROM fray_messages
		WHERE guid = ? OR reply_to = ?
		ORDER BY CASE WHEN guid = ? THEN 0 ELSE 1 END, ts ASC, guid ASC
	`, messageColumns), messageID, messageID, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessagesWithReactions(db, rows)
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

// GetReplies returns all direct replies to a message.
func GetReplies(db *sql.DB, messageID string) ([]types.Message, error) {
	rows, err := db.Query(fmt.Sprintf(`
		SELECT %s FROM fray_messages
		WHERE reply_to = ?
		ORDER BY ts ASC, guid ASC
	`, messageColumns), messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessagesWithReactions(db, rows)
}

type messageRow struct {
	GUID             string
	TS               int64
	ChannelID        sql.NullString
	Home             sql.NullString
	FromAgent        string
	Body             string
	Mentions         string
	Reactions        string
	MsgType          sql.NullString
	References       sql.NullString
	SurfaceMessage   sql.NullString
	ReplyTo          sql.NullString
	QuoteMessageGUID sql.NullString
	EditedAt         sql.NullInt64
	ArchivedAt       sql.NullInt64
}

func (row messageRow) toMessage() (types.Message, error) {
	mentions := []string{}
	if row.Mentions != "" {
		if err := json.Unmarshal([]byte(row.Mentions), &mentions); err != nil {
			return types.Message{}, err
		}
	}
	// Legacy reactions are stored as map[string][]string in the JSON column.
	// Convert them to the new format with ReactionEntry (with timestamp=0 for legacy).
	legacyReactions := map[string][]string{}
	if row.Reactions != "" {
		if err := json.Unmarshal([]byte(row.Reactions), &legacyReactions); err != nil {
			return types.Message{}, err
		}
	}
	reactions := ConvertLegacyReactions(normalizeReactionsLegacy(legacyReactions))

	msgType := types.MessageTypeAgent
	if row.MsgType.Valid && row.MsgType.String != "" {
		msgType = types.MessageType(row.MsgType.String)
	}
	home := "room"
	if row.Home.Valid && row.Home.String != "" {
		home = row.Home.String
	}

	return types.Message{
		ID:               row.GUID,
		TS:               row.TS,
		ChannelID:        nullStringPtr(row.ChannelID),
		Home:             home,
		FromAgent:        row.FromAgent,
		Body:             row.Body,
		Mentions:         mentions,
		Reactions:        reactions,
		Type:             msgType,
		References:       nullStringPtr(row.References),
		SurfaceMessage:   nullStringPtr(row.SurfaceMessage),
		ReplyTo:          nullStringPtr(row.ReplyTo),
		QuoteMessageGUID: nullStringPtr(row.QuoteMessageGUID),
		EditedAt:         nullIntPtr(row.EditedAt),
		ArchivedAt:       nullIntPtr(row.ArchivedAt),
	}, nil
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
	if err := scanner.Scan(&row.GUID, &row.TS, &row.ChannelID, &row.Home, &row.FromAgent, &row.Body, &row.Mentions, &row.MsgType, &row.References, &row.SurfaceMessage, &row.ReplyTo, &row.QuoteMessageGUID, &row.EditedAt, &row.ArchivedAt, &row.Reactions); err != nil {
		return types.Message{}, err
	}
	return row.toMessage()
}
