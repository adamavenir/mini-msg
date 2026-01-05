package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// ThreadUpdates represents partial thread updates.
type ThreadUpdates struct {
	Name              types.OptionalString
	Status            types.OptionalString
	Type              types.OptionalString
	ParentThread      types.OptionalString
	AnchorMessageGUID types.OptionalString
	AnchorHidden      types.OptionalBool
	LastActivityAt    types.OptionalInt64
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
	threadType := thread.Type
	if threadType == "" {
		threadType = types.ThreadTypeStandard
	}
	createdAt := thread.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	anchorHidden := 0
	if thread.AnchorHidden {
		anchorHidden = 1
	}

	_, err := db.Exec(`
		INSERT INTO fray_threads (guid, name, parent_thread, status, type, created_at, anchor_message_guid, anchor_hidden, last_activity_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, guid, thread.Name, thread.ParentThread, string(status), string(threadType), createdAt, thread.AnchorMessageGUID, anchorHidden, thread.LastActivityAt)
	if err != nil {
		return types.Thread{}, err
	}

	thread.GUID = guid
	thread.Status = status
	thread.Type = threadType
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
	if updates.Type.Set {
		fields = append(fields, "type = ?")
		args = append(args, nullableValue(updates.Type.Value))
	}
	if updates.ParentThread.Set {
		fields = append(fields, "parent_thread = ?")
		args = append(args, nullableValue(updates.ParentThread.Value))
	}
	if updates.AnchorMessageGUID.Set {
		fields = append(fields, "anchor_message_guid = ?")
		args = append(args, nullableValue(updates.AnchorMessageGUID.Value))
	}
	if updates.AnchorHidden.Set {
		fields = append(fields, "anchor_hidden = ?")
		if updates.AnchorHidden.Value {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if updates.LastActivityAt.Set {
		fields = append(fields, "last_activity_at = ?")
		args = append(args, updates.LastActivityAt.Value)
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
		SELECT guid, name, parent_thread, status, type, created_at, anchor_message_guid, anchor_hidden, last_activity_at
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
		SELECT guid, name, parent_thread, status, type, created_at, anchor_message_guid, anchor_hidden, last_activity_at
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
			SELECT guid, name, parent_thread, status, type, created_at, anchor_message_guid, anchor_hidden, last_activity_at
			FROM fray_threads WHERE name = ? AND parent_thread IS NULL
		`, name)
	} else {
		row = db.QueryRow(`
			SELECT guid, name, parent_thread, status, type, created_at, anchor_message_guid, anchor_hidden, last_activity_at
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
		SELECT DISTINCT t.guid, t.name, t.parent_thread, t.status, t.type, t.created_at, t.anchor_message_guid, t.anchor_hidden, t.last_activity_at
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
	if options != nil && options.SortByActivity {
		query += " ORDER BY COALESCE(t.last_activity_at, t.created_at) DESC"
	} else {
		query += " ORDER BY t.created_at ASC"
	}

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
	rows, err := db.Query(fmt.Sprintf(`
		SELECT DISTINCT %s FROM fray_messages m
		LEFT JOIN fray_thread_messages tm ON tm.message_guid = m.guid AND tm.thread_guid = ?
		WHERE m.home = ? OR tm.thread_guid = ?
		ORDER BY m.ts ASC, m.guid ASC
	`, messageColumnsAliased), threadGUID, threadGUID, threadGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessagesWithReactions(db, rows)
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

func scanThread(scanner interface{ Scan(dest ...any) error }) (types.Thread, error) {
	var row threadRow
	if err := scanner.Scan(&row.GUID, &row.Name, &row.ParentThread, &row.Status, &row.Type, &row.CreatedAt, &row.AnchorMessageGUID, &row.AnchorHidden, &row.LastActivityAt); err != nil {
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

type threadRow struct {
	GUID              string
	Name              string
	ParentThread      sql.NullString
	Status            sql.NullString
	Type              sql.NullString
	CreatedAt         int64
	AnchorMessageGUID sql.NullString
	AnchorHidden      sql.NullInt64
	LastActivityAt    sql.NullInt64
}

func (row threadRow) toThread() types.Thread {
	status := types.ThreadStatusOpen
	if row.Status.Valid && row.Status.String != "" {
		status = types.ThreadStatus(row.Status.String)
	}
	threadType := types.ThreadTypeStandard
	if row.Type.Valid && row.Type.String != "" {
		threadType = types.ThreadType(row.Type.String)
	}
	thread := types.Thread{
		GUID:         row.GUID,
		Name:         row.Name,
		ParentThread: nullStringPtr(row.ParentThread),
		Status:       status,
		Type:         threadType,
		CreatedAt:    row.CreatedAt,
	}
	if row.AnchorMessageGUID.Valid {
		thread.AnchorMessageGUID = &row.AnchorMessageGUID.String
	}
	if row.AnchorHidden.Valid && row.AnchorHidden.Int64 != 0 {
		thread.AnchorHidden = true
	}
	if row.LastActivityAt.Valid {
		thread.LastActivityAt = &row.LastActivityAt.Int64
	}
	return thread
}

// PinMessage pins a message within a thread.
func PinMessage(db *sql.DB, messageGUID, threadGUID, pinnedBy string, pinnedAt int64) error {
	if pinnedAt == 0 {
		pinnedAt = time.Now().Unix()
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_message_pins (message_guid, thread_guid, pinned_by, pinned_at)
		VALUES (?, ?, ?, ?)
	`, messageGUID, threadGUID, pinnedBy, pinnedAt)
	return err
}

// UnpinMessage unpins a message from a thread.
func UnpinMessage(db *sql.DB, messageGUID, threadGUID string) error {
	_, err := db.Exec(`
		DELETE FROM fray_message_pins WHERE message_guid = ? AND thread_guid = ?
	`, messageGUID, threadGUID)
	return err
}

// IsMessagePinned checks if a message is pinned in a thread.
func IsMessagePinned(db *sql.DB, messageGUID, threadGUID string) (bool, error) {
	row := db.QueryRow(`
		SELECT 1 FROM fray_message_pins WHERE message_guid = ? AND thread_guid = ?
	`, messageGUID, threadGUID)
	var value int
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetPinnedMessages returns messages pinned in a thread.
func GetPinnedMessages(db *sql.DB, threadGUID string) ([]types.Message, error) {
	rows, err := db.Query(fmt.Sprintf(`
		SELECT %s FROM fray_messages m
		INNER JOIN fray_message_pins p ON p.message_guid = m.guid
		WHERE p.thread_guid = ?
		ORDER BY p.pinned_at ASC
	`, messageColumnsAliased), threadGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessagesWithReactions(db, rows)
}

// MoveMessage changes a message's home field.
func MoveMessage(db *sql.DB, messageGUID, newHome string) error {
	_, err := db.Exec(`
		UPDATE fray_messages SET home = ? WHERE guid = ?
	`, newHome, messageGUID)
	return err
}

// UpdateThreadActivity sets last_activity_at for a thread.
func UpdateThreadActivity(db *sql.DB, threadGUID string, activityAt int64) error {
	_, err := db.Exec(`
		UPDATE fray_threads SET last_activity_at = ? WHERE guid = ?
	`, activityAt, threadGUID)
	return err
}

// GetPinnedMessageCount returns the number of pinned messages in a thread.
func GetPinnedMessageCount(db *sql.DB, threadGUID string) (int64, error) {
	row := db.QueryRow(`
		SELECT COUNT(*) FROM fray_message_pins WHERE thread_guid = ?
	`, threadGUID)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// PinThread pins a thread (public, visible to all).
func PinThread(db *sql.DB, threadGUID, pinnedBy string, pinnedAt int64) error {
	if pinnedAt == 0 {
		pinnedAt = time.Now().Unix()
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_thread_pins (thread_guid, pinned_by, pinned_at)
		VALUES (?, ?, ?)
	`, threadGUID, pinnedBy, pinnedAt)
	return err
}

// UnpinThread unpins a thread.
func UnpinThread(db *sql.DB, threadGUID string) error {
	_, err := db.Exec(`
		DELETE FROM fray_thread_pins WHERE thread_guid = ?
	`, threadGUID)
	return err
}

// IsThreadPinned checks if a thread is pinned.
func IsThreadPinned(db *sql.DB, threadGUID string) (bool, error) {
	row := db.QueryRow(`
		SELECT 1 FROM fray_thread_pins WHERE thread_guid = ?
	`, threadGUID)
	var value int
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetPinnedThreads returns all pinned threads.
func GetPinnedThreads(db *sql.DB) ([]types.Thread, error) {
	rows, err := db.Query(`
		SELECT t.guid, t.name, t.parent_thread, t.status, t.type, t.created_at, t.anchor_message_guid, t.anchor_hidden, t.last_activity_at
		FROM fray_threads t
		INNER JOIN fray_thread_pins p ON p.thread_guid = t.guid
		ORDER BY p.pinned_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanThreads(rows)
}

// MuteThread mutes a thread for an agent.
func MuteThread(db *sql.DB, threadGUID, agentID string, mutedAt int64, expiresAt *int64) error {
	if mutedAt == 0 {
		mutedAt = time.Now().Unix()
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_thread_mutes (thread_guid, agent_id, muted_at, expires_at)
		VALUES (?, ?, ?, ?)
	`, threadGUID, agentID, mutedAt, expiresAt)
	return err
}

// UnmuteThread unmutes a thread for an agent.
func UnmuteThread(db *sql.DB, threadGUID, agentID string) error {
	_, err := db.Exec(`
		DELETE FROM fray_thread_mutes WHERE thread_guid = ? AND agent_id = ?
	`, threadGUID, agentID)
	return err
}

// IsThreadMuted checks if a thread is muted for an agent (respects expiry).
func IsThreadMuted(db *sql.DB, threadGUID, agentID string) (bool, error) {
	now := time.Now().Unix()
	row := db.QueryRow(`
		SELECT 1 FROM fray_thread_mutes
		WHERE thread_guid = ? AND agent_id = ?
		AND (expires_at IS NULL OR expires_at > ?)
	`, threadGUID, agentID, now)
	var value int
	if err := row.Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetMutedThreads returns threads muted by an agent (respects expiry).
func GetMutedThreads(db *sql.DB, agentID string) ([]types.Thread, error) {
	now := time.Now().Unix()
	rows, err := db.Query(`
		SELECT t.guid, t.name, t.parent_thread, t.status, t.type, t.created_at, t.anchor_message_guid, t.anchor_hidden, t.last_activity_at
		FROM fray_threads t
		INNER JOIN fray_thread_mutes m ON m.thread_guid = t.guid
		WHERE m.agent_id = ?
		AND (m.expires_at IS NULL OR m.expires_at > ?)
		ORDER BY m.muted_at DESC
	`, agentID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanThreads(rows)
}

// GetMutedThreadGUIDs returns GUIDs of threads muted by an agent (respects expiry).
func GetMutedThreadGUIDs(db *sql.DB, agentID string) (map[string]bool, error) {
	now := time.Now().Unix()
	rows, err := db.Query(`
		SELECT thread_guid FROM fray_thread_mutes
		WHERE agent_id = ?
		AND (expires_at IS NULL OR expires_at > ?)
	`, agentID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	muted := make(map[string]bool)
	for rows.Next() {
		var guid string
		if err := rows.Scan(&guid); err != nil {
			return nil, err
		}
		muted[guid] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return muted, nil
}
