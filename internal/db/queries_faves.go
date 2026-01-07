package db

import (
	"database/sql"
	"time"
)

// Fave represents a faved item.
type Fave struct {
	AgentID  string
	ItemType string // "thread" | "message"
	ItemGUID string
	FavedAt  int64
}

// AddFave adds a fave for an agent.
func AddFave(db *sql.DB, agentID, itemType, itemGUID string) (int64, error) {
	favedAt := time.Now().UnixMilli()
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_faves (agent_id, item_type, item_guid, faved_at)
		VALUES (?, ?, ?, ?)
	`, agentID, itemType, itemGUID, favedAt)
	return favedAt, err
}

// RemoveFave removes a fave for an agent.
func RemoveFave(db *sql.DB, agentID, itemType, itemGUID string) error {
	_, err := db.Exec(`
		DELETE FROM fray_faves
		WHERE agent_id = ? AND item_type = ? AND item_guid = ?
	`, agentID, itemType, itemGUID)
	return err
}

// IsFaved checks if an item is faved by an agent.
func IsFaved(db *sql.DB, agentID, itemType, itemGUID string) (bool, error) {
	row := db.QueryRow(`
		SELECT 1 FROM fray_faves
		WHERE agent_id = ? AND item_type = ? AND item_guid = ?
	`, agentID, itemType, itemGUID)
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

// GetFaves returns all faves for an agent.
func GetFaves(db *sql.DB, agentID string, itemType string) ([]Fave, error) {
	query := `
		SELECT agent_id, item_type, item_guid, faved_at
		FROM fray_faves
		WHERE agent_id = ?
	`
	params := []any{agentID}

	if itemType != "" {
		query += " AND item_type = ?"
		params = append(params, itemType)
	}

	query += " ORDER BY faved_at DESC"

	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var faves []Fave
	for rows.Next() {
		var f Fave
		if err := rows.Scan(&f.AgentID, &f.ItemType, &f.ItemGUID, &f.FavedAt); err != nil {
			return nil, err
		}
		faves = append(faves, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return faves, nil
}

// GetFavedThreads returns thread GUIDs that an agent has faved.
func GetFavedThreads(db *sql.DB, agentID string) ([]string, error) {
	rows, err := db.Query(`
		SELECT item_guid FROM fray_faves
		WHERE agent_id = ? AND item_type = 'thread' AND faved_at IS NOT NULL
		ORDER BY faved_at DESC
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guids []string
	for rows.Next() {
		var guid string
		if err := rows.Scan(&guid); err != nil {
			return nil, err
		}
		guids = append(guids, guid)
	}
	return guids, rows.Err()
}

// SetNickname sets a nickname for a thread or message (without faving it).
func SetNickname(db *sql.DB, agentID, itemType, itemGUID, nickname string) error {
	_, err := db.Exec(`
		INSERT INTO fray_faves (agent_id, item_type, item_guid, nickname)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(agent_id, item_type, item_guid)
		DO UPDATE SET nickname = ?
	`, agentID, itemType, itemGUID, nickname, nickname)
	return err
}

// GetNickname retrieves the nickname for a thread or message.
func GetNickname(db *sql.DB, agentID, itemType, itemGUID string) (string, error) {
	var nickname sql.NullString
	err := db.QueryRow(`
		SELECT nickname FROM fray_faves
		WHERE agent_id = ? AND item_type = ? AND item_guid = ?
	`, agentID, itemType, itemGUID).Scan(&nickname)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return nickname.String, nil
}

// GetThreadNicknames returns all thread nicknames for an agent.
func GetThreadNicknames(db *sql.DB, agentID string) (map[string]string, error) {
	rows, err := db.Query(`
		SELECT item_guid, nickname FROM fray_faves
		WHERE agent_id = ? AND item_type = 'thread' AND nickname IS NOT NULL
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nicknames := make(map[string]string)
	for rows.Next() {
		var guid string
		var nickname sql.NullString
		if err := rows.Scan(&guid, &nickname); err != nil {
			return nil, err
		}
		if nickname.Valid && nickname.String != "" {
			nicknames[guid] = nickname.String
		}
	}
	return nicknames, rows.Err()
}
