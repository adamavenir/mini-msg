package db

import (
	"database/sql"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// AddRoleAssignment adds a persistent role to an agent.
func AddRoleAssignment(db *sql.DB, agentID, roleName string) error {
	now := time.Now().Unix()
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_role_assignments (agent_id, role_name, assigned_at)
		VALUES (?, ?, ?)
	`, agentID, roleName, now)
	return err
}

// RemoveRoleAssignment removes a persistent role from an agent.
func RemoveRoleAssignment(db *sql.DB, agentID, roleName string) (bool, error) {
	result, err := db.Exec(`
		DELETE FROM fray_role_assignments
		WHERE agent_id = ? AND role_name = ?
	`, agentID, roleName)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetRoleAssignments returns all roles held by an agent.
func GetRoleAssignments(db *sql.DB, agentID string) ([]types.RoleAssignment, error) {
	rows, err := db.Query(`
		SELECT agent_id, role_name, assigned_at
		FROM fray_role_assignments
		WHERE agent_id = ?
		ORDER BY assigned_at
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []types.RoleAssignment
	for rows.Next() {
		var a types.RoleAssignment
		if err := rows.Scan(&a.AgentID, &a.RoleName, &a.AssignedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

// GetAgentsByRole returns all agents holding a role.
func GetAgentsByRole(db *sql.DB, roleName string) ([]types.RoleAssignment, error) {
	rows, err := db.Query(`
		SELECT agent_id, role_name, assigned_at
		FROM fray_role_assignments
		WHERE role_name = ?
		ORDER BY assigned_at
	`, roleName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []types.RoleAssignment
	for rows.Next() {
		var a types.RoleAssignment
		if err := rows.Scan(&a.AgentID, &a.RoleName, &a.AssignedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

// HasRoleAssignment checks if an agent holds a specific role.
func HasRoleAssignment(db *sql.DB, agentID, roleName string) (bool, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM fray_role_assignments
		WHERE agent_id = ? AND role_name = ?
	`, agentID, roleName).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddSessionRole adds a session-scoped role to an agent.
func AddSessionRole(db *sql.DB, agentID, roleName string, sessionID *string) error {
	now := time.Now().Unix()
	_, err := db.Exec(`
		INSERT OR REPLACE INTO fray_session_roles (agent_id, role_name, session_id, started_at)
		VALUES (?, ?, ?, ?)
	`, agentID, roleName, sessionID, now)
	return err
}

// RemoveSessionRole removes a session-scoped role from an agent.
func RemoveSessionRole(db *sql.DB, agentID, roleName string) (bool, error) {
	result, err := db.Exec(`
		DELETE FROM fray_session_roles
		WHERE agent_id = ? AND role_name = ?
	`, agentID, roleName)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ClearSessionRoles removes all session-scoped roles for an agent.
func ClearSessionRoles(db *sql.DB, agentID string) (int64, error) {
	result, err := db.Exec(`
		DELETE FROM fray_session_roles WHERE agent_id = ?
	`, agentID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetSessionRoles returns all session-scoped roles for an agent.
func GetSessionRoles(db *sql.DB, agentID string) ([]types.SessionRole, error) {
	rows, err := db.Query(`
		SELECT agent_id, role_name, session_id, started_at
		FROM fray_session_roles
		WHERE agent_id = ?
		ORDER BY started_at
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []types.SessionRole
	for rows.Next() {
		var r types.SessionRole
		var sessionID sql.NullString
		if err := rows.Scan(&r.AgentID, &r.RoleName, &sessionID, &r.StartedAt); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			r.SessionID = &sessionID.String
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// HasSessionRole checks if an agent is playing a specific role.
func HasSessionRole(db *sql.DB, agentID, roleName string) (bool, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM fray_session_roles
		WHERE agent_id = ? AND role_name = ?
	`, agentID, roleName).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetAgentRoles returns combined held and playing roles for an agent.
func GetAgentRoles(db *sql.DB, agentID string) (*types.AgentRoles, error) {
	result := &types.AgentRoles{
		AgentID: agentID,
		Held:    []string{},
		Playing: []string{},
	}

	// Get held roles
	assignments, err := GetRoleAssignments(db, agentID)
	if err != nil {
		return nil, err
	}
	for _, a := range assignments {
		result.Held = append(result.Held, a.RoleName)
	}

	// Get session roles
	sessionRoles, err := GetSessionRoles(db, agentID)
	if err != nil {
		return nil, err
	}
	for _, r := range sessionRoles {
		result.Playing = append(result.Playing, r.RoleName)
	}

	return result, nil
}

// GetAllRoles returns all unique role names in use.
func GetAllRoles(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT DISTINCT role_name FROM (
			SELECT role_name FROM fray_role_assignments
			UNION
			SELECT role_name FROM fray_session_roles
		)
		ORDER BY role_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

// GetAllAgentRoles returns roles for all agents with roles.
func GetAllAgentRoles(db *sql.DB) (map[string]*types.AgentRoles, error) {
	result := make(map[string]*types.AgentRoles)

	// Get all held roles
	rows, err := db.Query(`
		SELECT agent_id, role_name FROM fray_role_assignments ORDER BY agent_id, assigned_at
	`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var agentID, roleName string
		if err := rows.Scan(&agentID, &roleName); err != nil {
			rows.Close()
			return nil, err
		}
		if result[agentID] == nil {
			result[agentID] = &types.AgentRoles{AgentID: agentID, Held: []string{}, Playing: []string{}}
		}
		result[agentID].Held = append(result[agentID].Held, roleName)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get all session roles
	rows, err = db.Query(`
		SELECT agent_id, role_name FROM fray_session_roles ORDER BY agent_id, started_at
	`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var agentID, roleName string
		if err := rows.Scan(&agentID, &roleName); err != nil {
			rows.Close()
			return nil, err
		}
		if result[agentID] == nil {
			result[agentID] = &types.AgentRoles{AgentID: agentID, Held: []string{}, Playing: []string{}}
		}
		result[agentID].Playing = append(result[agentID].Playing, roleName)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
