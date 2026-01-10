package db

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
)

// CreateWakeCondition creates a new wake condition in the database and JSONL.
func CreateWakeCondition(db *sql.DB, projectPath string, input types.WakeConditionInput) (*types.WakeCondition, error) {
	guid, err := core.GenerateGUID("wake")
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	condition := &types.WakeCondition{
		GUID:      guid,
		AgentID:   input.AgentID,
		SetBy:     input.SetBy,
		Type:      input.Type,
		Pattern:   input.Pattern,
		OnAgents:  input.OnAgents,
		InThread:  input.InThread,
		AfterMs:   input.AfterMs,
		UseRouter: input.UseRouter,
		Prompt:    input.Prompt,
		CreatedAt: now,
	}

	// Calculate expiry for "after" conditions
	if input.Type == types.WakeConditionAfter && input.AfterMs != nil {
		expiresAt := now + (*input.AfterMs / 1000)
		condition.ExpiresAt = &expiresAt
	}

	// Marshal JSON fields
	onAgentsJSON, err := json.Marshal(input.OnAgents)
	if err != nil {
		return nil, err
	}

	// Insert into database
	_, err = db.Exec(`
		INSERT INTO fray_wake_conditions (
			guid, agent_id, set_by, type, pattern, on_agents, in_thread,
			after_ms, use_router, prompt, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		condition.GUID,
		condition.AgentID,
		condition.SetBy,
		string(condition.Type),
		condition.Pattern,
		string(onAgentsJSON),
		condition.InThread,
		condition.AfterMs,
		condition.UseRouter,
		condition.Prompt,
		condition.CreatedAt,
		condition.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}

	// Append to JSONL
	if err := AppendWakeCondition(projectPath, *condition); err != nil {
		return nil, err
	}

	return condition, nil
}

// GetWakeConditions retrieves wake conditions, optionally filtered by agent.
func GetWakeConditions(db *sql.DB, agentID string) ([]types.WakeCondition, error) {
	query := `
		SELECT guid, agent_id, set_by, type, pattern, on_agents, in_thread,
		       after_ms, use_router, prompt, created_at, expires_at
		FROM fray_wake_conditions
		WHERE (expires_at IS NULL OR expires_at > ?)
	`
	args := []any{time.Now().Unix()}

	if agentID != "" {
		query += " AND agent_id = ?"
		args = append(args, agentID)
	}

	query += " ORDER BY created_at ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conditions []types.WakeCondition
	for rows.Next() {
		var c types.WakeCondition
		var condType string
		var onAgentsJSON string
		var pattern, inThread, prompt sql.NullString
		var afterMs, expiresAt sql.NullInt64

		err := rows.Scan(
			&c.GUID,
			&c.AgentID,
			&c.SetBy,
			&condType,
			&pattern,
			&onAgentsJSON,
			&inThread,
			&afterMs,
			&c.UseRouter,
			&prompt,
			&c.CreatedAt,
			&expiresAt,
		)
		if err != nil {
			return nil, err
		}

		c.Type = types.WakeConditionType(condType)

		if pattern.Valid {
			c.Pattern = &pattern.String
		}
		if inThread.Valid {
			c.InThread = &inThread.String
		}
		if afterMs.Valid {
			c.AfterMs = &afterMs.Int64
		}
		if prompt.Valid {
			c.Prompt = &prompt.String
		}
		if expiresAt.Valid {
			c.ExpiresAt = &expiresAt.Int64
		}

		if onAgentsJSON != "" && onAgentsJSON != "null" {
			if err := json.Unmarshal([]byte(onAgentsJSON), &c.OnAgents); err != nil {
				c.OnAgents = nil
			}
		}

		conditions = append(conditions, c)
	}

	return conditions, rows.Err()
}

// GetWakeCondition retrieves a single wake condition by GUID.
func GetWakeCondition(db *sql.DB, guid string) (*types.WakeCondition, error) {
	var c types.WakeCondition
	var condType string
	var onAgentsJSON string
	var pattern, inThread, prompt sql.NullString
	var afterMs, expiresAt sql.NullInt64

	err := db.QueryRow(`
		SELECT guid, agent_id, set_by, type, pattern, on_agents, in_thread,
		       after_ms, use_router, prompt, created_at, expires_at
		FROM fray_wake_conditions
		WHERE guid = ?
	`, guid).Scan(
		&c.GUID,
		&c.AgentID,
		&c.SetBy,
		&condType,
		&pattern,
		&onAgentsJSON,
		&inThread,
		&afterMs,
		&c.UseRouter,
		&prompt,
		&c.CreatedAt,
		&expiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.Type = types.WakeConditionType(condType)

	if pattern.Valid {
		c.Pattern = &pattern.String
	}
	if inThread.Valid {
		c.InThread = &inThread.String
	}
	if afterMs.Valid {
		c.AfterMs = &afterMs.Int64
	}
	if prompt.Valid {
		c.Prompt = &prompt.String
	}
	if expiresAt.Valid {
		c.ExpiresAt = &expiresAt.Int64
	}

	if onAgentsJSON != "" && onAgentsJSON != "null" {
		if err := json.Unmarshal([]byte(onAgentsJSON), &c.OnAgents); err != nil {
			c.OnAgents = nil
		}
	}

	return &c, nil
}

// ClearWakeConditions removes all wake conditions for an agent.
func ClearWakeConditions(db *sql.DB, projectPath string, agentID string) (int64, error) {
	result, err := db.Exec(`DELETE FROM fray_wake_conditions WHERE agent_id = ?`, agentID)
	if err != nil {
		return 0, err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	// Append clear record to JSONL
	if count > 0 {
		if err := AppendWakeConditionClear(projectPath, agentID); err != nil {
			return count, err
		}
	}

	return count, nil
}

// DeleteWakeCondition removes a specific wake condition.
func DeleteWakeCondition(db *sql.DB, projectPath string, guid string) error {
	_, err := db.Exec(`DELETE FROM fray_wake_conditions WHERE guid = ?`, guid)
	if err != nil {
		return err
	}

	// Append delete record to JSONL
	return AppendWakeConditionDelete(projectPath, guid)
}

// GetPatternWakeConditions retrieves all pattern-based wake conditions that are active.
func GetPatternWakeConditions(db *sql.DB) ([]types.WakeCondition, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, set_by, type, pattern, on_agents, in_thread,
		       after_ms, use_router, prompt, created_at, expires_at
		FROM fray_wake_conditions
		WHERE type = ?
		  AND (expires_at IS NULL OR expires_at > ?)
	`, string(types.WakeConditionPattern), time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conditions []types.WakeCondition
	for rows.Next() {
		var c types.WakeCondition
		var condType string
		var onAgentsJSON string
		var pattern, inThread, prompt sql.NullString
		var afterMs, expiresAt sql.NullInt64

		err := rows.Scan(
			&c.GUID,
			&c.AgentID,
			&c.SetBy,
			&condType,
			&pattern,
			&onAgentsJSON,
			&inThread,
			&afterMs,
			&c.UseRouter,
			&prompt,
			&c.CreatedAt,
			&expiresAt,
		)
		if err != nil {
			return nil, err
		}

		c.Type = types.WakeConditionType(condType)

		if pattern.Valid {
			c.Pattern = &pattern.String
		}
		if inThread.Valid {
			c.InThread = &inThread.String
		}
		if afterMs.Valid {
			c.AfterMs = &afterMs.Int64
		}
		if prompt.Valid {
			c.Prompt = &prompt.String
		}
		if expiresAt.Valid {
			c.ExpiresAt = &expiresAt.Int64
		}

		if onAgentsJSON != "" && onAgentsJSON != "null" {
			if err := json.Unmarshal([]byte(onAgentsJSON), &c.OnAgents); err != nil {
				c.OnAgents = nil
			}
		}

		conditions = append(conditions, c)
	}

	return conditions, rows.Err()
}

// GetExpiredWakeConditions retrieves wake conditions that have expired.
func GetExpiredWakeConditions(db *sql.DB) ([]types.WakeCondition, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, set_by, type, pattern, on_agents, in_thread,
		       after_ms, use_router, prompt, created_at, expires_at
		FROM fray_wake_conditions
		WHERE expires_at IS NOT NULL AND expires_at <= ?
	`, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conditions []types.WakeCondition
	for rows.Next() {
		var c types.WakeCondition
		var condType string
		var onAgentsJSON string
		var pattern, inThread, prompt sql.NullString
		var afterMs, expiresAt sql.NullInt64

		err := rows.Scan(
			&c.GUID,
			&c.AgentID,
			&c.SetBy,
			&condType,
			&pattern,
			&onAgentsJSON,
			&inThread,
			&afterMs,
			&c.UseRouter,
			&prompt,
			&c.CreatedAt,
			&expiresAt,
		)
		if err != nil {
			return nil, err
		}

		c.Type = types.WakeConditionType(condType)

		if pattern.Valid {
			c.Pattern = &pattern.String
		}
		if inThread.Valid {
			c.InThread = &inThread.String
		}
		if afterMs.Valid {
			c.AfterMs = &afterMs.Int64
		}
		if prompt.Valid {
			c.Prompt = &prompt.String
		}
		if expiresAt.Valid {
			c.ExpiresAt = &expiresAt.Int64
		}

		if onAgentsJSON != "" && onAgentsJSON != "null" {
			if err := json.Unmarshal([]byte(onAgentsJSON), &c.OnAgents); err != nil {
				c.OnAgents = nil
			}
		}

		conditions = append(conditions, c)
	}

	return conditions, rows.Err()
}

// PruneExpiredWakeConditions removes expired wake conditions.
func PruneExpiredWakeConditions(db *sql.DB) (int64, error) {
	result, err := db.Exec(`
		DELETE FROM fray_wake_conditions
		WHERE expires_at IS NOT NULL AND expires_at <= ?
	`, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
