package db

import (
	"database/sql"

	"github.com/adamavenir/fray/internal/types"
)

// SetGhostCursor sets or updates a ghost cursor for an agent in a specific home.
// Note: This clears session_ack_at (new cursor = new handoff, needs fresh ack).
func SetGhostCursor(db *sql.DB, cursor types.GhostCursor) error {
	mustRead := 0
	if cursor.MustRead {
		mustRead = 1
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_ghost_cursors (agent_id, home, message_guid, must_read, set_at, session_ack_at)
		VALUES (?, ?, ?, ?, ?, NULL)
	`, cursor.AgentID, cursor.Home, cursor.MessageGUID, mustRead, cursor.SetAt)
	return err
}

// GetGhostCursor retrieves a ghost cursor for an agent in a specific home.
func GetGhostCursor(db *sql.DB, agentID, home string) (*types.GhostCursor, error) {
	row := db.QueryRow(`
		SELECT agent_id, home, message_guid, must_read, set_at, session_ack_at
		FROM fray_ghost_cursors
		WHERE agent_id = ? AND home = ?
	`, agentID, home)

	var cursor types.GhostCursor
	var mustRead int
	var sessionAckAt sql.NullInt64
	err := row.Scan(&cursor.AgentID, &cursor.Home, &cursor.MessageGUID, &mustRead, &cursor.SetAt, &sessionAckAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cursor.MustRead = mustRead != 0
	if sessionAckAt.Valid {
		cursor.SessionAckAt = &sessionAckAt.Int64
	}
	return &cursor, nil
}

// GetGhostCursors retrieves all ghost cursors for an agent.
func GetGhostCursors(db *sql.DB, agentID string) ([]types.GhostCursor, error) {
	rows, err := db.Query(`
		SELECT agent_id, home, message_guid, must_read, set_at, session_ack_at
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
		var sessionAckAt sql.NullInt64
		if err := rows.Scan(&cursor.AgentID, &cursor.Home, &cursor.MessageGUID, &mustRead, &cursor.SetAt, &sessionAckAt); err != nil {
			return nil, err
		}
		cursor.MustRead = mustRead != 0
		if sessionAckAt.Valid {
			cursor.SessionAckAt = &sessionAckAt.Int64
		}
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
		SELECT agent_id, home, message_guid, must_read, set_at, session_ack_at
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
		var sessionAckAt sql.NullInt64
		if err := rows.Scan(&cursor.AgentID, &cursor.Home, &cursor.MessageGUID, &mustRead, &cursor.SetAt, &sessionAckAt); err != nil {
			return nil, err
		}
		cursor.MustRead = mustRead != 0
		if sessionAckAt.Valid {
			cursor.SessionAckAt = &sessionAckAt.Int64
		}
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

// AckGhostCursor marks a ghost cursor as acknowledged for this session.
// Call this when the agent first views content past the ghost cursor.
func AckGhostCursor(db *sql.DB, agentID, home string, ackAt int64) error {
	_, err := db.Exec(`
		UPDATE fray_ghost_cursors
		SET session_ack_at = ?
		WHERE agent_id = ? AND home = ? AND session_ack_at IS NULL
	`, ackAt, agentID, home)
	return err
}

// ClearGhostCursorSessionAcks clears session_ack_at for all ghost cursors of an agent.
// Call this on session start (fray new/back) so cursors become unread again.
func ClearGhostCursorSessionAcks(db *sql.DB, agentID string) error {
	_, err := db.Exec(`
		UPDATE fray_ghost_cursors
		SET session_ack_at = NULL
		WHERE agent_id = ?
	`, agentID)
	return err
}
