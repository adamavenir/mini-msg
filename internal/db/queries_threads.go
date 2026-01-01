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
