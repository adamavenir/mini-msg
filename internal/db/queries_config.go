package db

import (
	"database/sql"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

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

// ReadTo represents a watermark position for an agent in a context.
type ReadTo struct {
	AgentID     string `json:"agent_id"`
	Home        string `json:"home"`
	MessageGUID string `json:"message_guid"`
	MessageTS   int64  `json:"message_ts"`
	SetAt       int64  `json:"set_at"`
}

// SetReadTo sets or updates an agent's read watermark for a context.
func SetReadTo(db *sql.DB, agentID, home, messageGUID string, messageTS int64) error {
	now := time.Now().Unix()
	_, err := db.Exec(`
		INSERT INTO fray_read_to (agent_id, home, message_guid, message_ts, set_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, home) DO UPDATE SET
			message_guid = excluded.message_guid,
			message_ts = excluded.message_ts,
			set_at = excluded.set_at
		WHERE excluded.message_ts > fray_read_to.message_ts
	`, agentID, home, messageGUID, messageTS, now)
	return err
}

// GetReadTo returns an agent's read watermark for a context.
func GetReadTo(db *sql.DB, agentID, home string) (*ReadTo, error) {
	row := db.QueryRow(`
		SELECT agent_id, home, message_guid, message_ts, set_at
		FROM fray_read_to
		WHERE agent_id = ? AND home = ?
	`, agentID, home)
	var r ReadTo
	if err := row.Scan(&r.AgentID, &r.Home, &r.MessageGUID, &r.MessageTS, &r.SetAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// GetReadToForHome returns all agents' read watermarks for a context.
func GetReadToForHome(db *sql.DB, home string) ([]ReadTo, error) {
	rows, err := db.Query(`
		SELECT agent_id, home, message_guid, message_ts, set_at
		FROM fray_read_to
		WHERE home = ?
		ORDER BY message_ts DESC
	`, home)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ReadTo
	for rows.Next() {
		var r ReadTo
		if err := rows.Scan(&r.AgentID, &r.Home, &r.MessageGUID, &r.MessageTS, &r.SetAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetReadToByMessage returns agents who have read up to a specific message.
func GetReadToByMessage(db *sql.DB, home, messageGUID string) ([]string, error) {
	rows, err := db.Query(`
		SELECT agent_id FROM fray_read_to
		WHERE home = ? AND message_guid = ?
	`, home, messageGUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []string
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return nil, err
		}
		agents = append(agents, agentID)
	}
	return agents, rows.Err()
}
