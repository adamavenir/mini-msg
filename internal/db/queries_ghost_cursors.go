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

// Staged cursor functions (session-temporary, not persisted to JSONL)

// SetStagedCursor sets or updates a staged cursor for an agent in a specific home.
func SetStagedCursor(db *sql.DB, cursor types.GhostCursor) error {
	mustRead := 0
	if cursor.MustRead {
		mustRead = 1
	}
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_staged_cursors (agent_id, home, message_guid, must_read, set_at)
		VALUES (?, ?, ?, ?, ?)
	`, cursor.AgentID, cursor.Home, cursor.MessageGUID, mustRead, cursor.SetAt)
	return err
}

// GetStagedCursors retrieves all staged cursors for an agent.
func GetStagedCursors(db *sql.DB, agentID string) ([]types.GhostCursor, error) {
	rows, err := db.Query(`
		SELECT agent_id, home, message_guid, must_read, set_at
		FROM fray_staged_cursors
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

// CommitStagedCursors moves all staged cursors to official ghost cursors.
// Returns the number of cursors committed.
func CommitStagedCursors(db *sql.DB, agentID string) (int, error) {
	// Insert staged cursors as ghost cursors
	result, err := db.Exec(`
		INSERT OR REPLACE INTO fray_ghost_cursors (agent_id, home, message_guid, must_read, set_at, session_ack_at)
		SELECT agent_id, home, message_guid, must_read, set_at, NULL
		FROM fray_staged_cursors
		WHERE agent_id = ?
	`, agentID)
	if err != nil {
		return 0, err
	}

	count, _ := result.RowsAffected()

	// Clear staged cursors
	_, err = db.Exec(`
		DELETE FROM fray_staged_cursors
		WHERE agent_id = ?
	`, agentID)
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// DeleteAllStagedCursors removes all staged cursors for an agent.
func DeleteAllStagedCursors(db *sql.DB, agentID string) error {
	_, err := db.Exec(`
		DELETE FROM fray_staged_cursors
		WHERE agent_id = ?
	`, agentID)
	return err
}
