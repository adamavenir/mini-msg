package db

import (
	"database/sql"

	"github.com/adamavenir/fray/internal/types"
)

// SetGhostCursor sets or updates a ghost cursor for an agent in a specific home.
func SetGhostCursor(db *sql.DB, cursor types.GhostCursor) error {
	mustRead := 0
	if cursor.MustRead {
		mustRead = 1
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_ghost_cursors (agent_id, home, message_guid, must_read, set_at)
		VALUES (?, ?, ?, ?, ?)
	`, cursor.AgentID, cursor.Home, cursor.MessageGUID, mustRead, cursor.SetAt)
	return err
}

// GetGhostCursor retrieves a ghost cursor for an agent in a specific home.
func GetGhostCursor(db *sql.DB, agentID, home string) (*types.GhostCursor, error) {
	row := db.QueryRow(`
		SELECT agent_id, home, message_guid, must_read, set_at
		FROM fray_ghost_cursors
		WHERE agent_id = ? AND home = ?
	`, agentID, home)

	var cursor types.GhostCursor
	var mustRead int
	err := row.Scan(&cursor.AgentID, &cursor.Home, &cursor.MessageGUID, &mustRead, &cursor.SetAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cursor.MustRead = mustRead != 0
	return &cursor, nil
}

// GetGhostCursors retrieves all ghost cursors for an agent.
func GetGhostCursors(db *sql.DB, agentID string) ([]types.GhostCursor, error) {
	rows, err := db.Query(`
		SELECT agent_id, home, message_guid, must_read, set_at
		FROM fray_ghost_cursors
		WHERE agent_id = ?
		ORDER BY home
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cursors []types.GhostCursor
	for rows.Next() {
		var cursor types.GhostCursor
		var mustRead int
		if err := rows.Scan(&cursor.AgentID, &cursor.Home, &cursor.MessageGUID, &mustRead, &cursor.SetAt); err != nil {
			return nil, err
		}
		cursor.MustRead = mustRead != 0
		cursors = append(cursors, cursor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cursors, nil
}

// GetMustReadCursors retrieves ghost cursors with must_read=true for an agent.
func GetMustReadCursors(db *sql.DB, agentID string) ([]types.GhostCursor, error) {
	rows, err := db.Query(`
		SELECT agent_id, home, message_guid, must_read, set_at
		FROM fray_ghost_cursors
		WHERE agent_id = ? AND must_read = 1
		ORDER BY home
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cursors []types.GhostCursor
	for rows.Next() {
		var cursor types.GhostCursor
		var mustRead int
		if err := rows.Scan(&cursor.AgentID, &cursor.Home, &cursor.MessageGUID, &mustRead, &cursor.SetAt); err != nil {
			return nil, err
		}
		cursor.MustRead = mustRead != 0
		cursors = append(cursors, cursor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cursors, nil
}

// DeleteGhostCursor removes a ghost cursor for an agent in a specific home.
func DeleteGhostCursor(db *sql.DB, agentID, home string) error {
	_, err := db.Exec(`
		DELETE FROM fray_ghost_cursors
		WHERE agent_id = ? AND home = ?
	`, agentID, home)
	return err
}

// DeleteAllGhostCursors removes all ghost cursors for an agent.
func DeleteAllGhostCursors(db *sql.DB, agentID string) error {
	_, err := db.Exec(`
		DELETE FROM fray_ghost_cursors
		WHERE agent_id = ?
	`, agentID)
	return err
}
