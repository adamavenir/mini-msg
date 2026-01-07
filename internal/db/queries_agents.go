package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/types"
)

// AgentUpdates represents partial agent updates.
type AgentUpdates struct {
	Status   types.OptionalString
	Purpose  types.OptionalString
	Avatar   types.OptionalString
	LastSeen types.OptionalInt64
	LeftAt   types.OptionalInt64
}

// GetAgent returns an agent by exact ID.
func GetAgent(db *sql.DB, agentID string) (*types.Agent, error) {
	row := db.QueryRow(`
		SELECT guid, agent_id, status, purpose, avatar, registered_at, last_seen, left_at, managed, invoke, presence, mention_watermark, last_heartbeat, last_session_id
		FROM fray_agents
		WHERE agent_id = ?
	`, agentID)

	agent, err := scanAgent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

// GetAgentsByPrefix returns agents matching a prefix.
func GetAgentsByPrefix(db *sql.DB, prefix string) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, avatar, registered_at, last_seen, left_at, managed, invoke, presence, mention_watermark, last_heartbeat, last_session_id
		FROM fray_agents
		WHERE agent_id = ? OR agent_id LIKE ?
		ORDER BY agent_id
	`, prefix, fmt.Sprintf("%s.%%", prefix))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []types.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetAgents returns all agents.
func GetAgents(db *sql.DB) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, avatar, registered_at, last_seen, left_at, managed, invoke, presence, mention_watermark, last_heartbeat, last_session_id
		FROM fray_agents
		ORDER BY agent_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []types.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

// CreateAgent inserts a new agent.
func CreateAgent(db *sql.DB, agent types.Agent) error {
	guid := agent.GUID
	if guid == "" {
		var err error
		guid, err = generateUniqueGUIDForTable(db, "fray_agents", "usr")
		if err != nil {
			return err
		}
	}

	var invokeJSON *string
	if agent.Invoke != nil {
		data, err := json.Marshal(agent.Invoke)
		if err != nil {
			return err
		}
		s := string(data)
		invokeJSON = &s
	}

	managed := 0
	if agent.Managed {
		managed = 1
	}

	presence := string(agent.Presence)
	if presence == "" {
		presence = string(types.PresenceOffline)
	}

	_, err := db.Exec(`
		INSERT INTO fray_agents (guid, agent_id, status, purpose, avatar, registered_at, last_seen, left_at, managed, invoke, presence, mention_watermark, last_heartbeat)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, guid, agent.AgentID, agent.Status, agent.Purpose, agent.Avatar, agent.RegisteredAt, agent.LastSeen, agent.LeftAt, managed, invokeJSON, presence, agent.MentionWatermark, agent.LastHeartbeat)
	return err
}

// UpdateAgentWatermark updates the mention watermark for an agent.
func UpdateAgentWatermark(db *sql.DB, agentID, msgID string) error {
	_, err := db.Exec(`UPDATE fray_agents SET mention_watermark = ? WHERE agent_id = ?`, msgID, agentID)
	return err
}

// UpdateAgentPresence updates the presence state for an agent.
func UpdateAgentPresence(db *sql.DB, agentID string, presence types.PresenceState) error {
	_, err := db.Exec(`UPDATE fray_agents SET presence = ? WHERE agent_id = ?`, string(presence), agentID)
	return err
}

// UpdateAgentHeartbeat updates the last heartbeat timestamp for an agent.
func UpdateAgentHeartbeat(db *sql.DB, agentID string, timestampMs int64) error {
	_, err := db.Exec(`UPDATE fray_agents SET last_heartbeat = ? WHERE agent_id = ?`, timestampMs, agentID)
	return err
}

// UpdateAgentSessionID updates the Claude Code session ID for resume.
func UpdateAgentSessionID(db *sql.DB, agentID, sessionID string) error {
	_, err := db.Exec(`UPDATE fray_agents SET last_session_id = ? WHERE agent_id = ?`, sessionID, agentID)
	return err
}

// UpdateAgent updates agent fields.
func UpdateAgent(db *sql.DB, agentID string, updates AgentUpdates) error {
	var fields []string
	var args []any

	if updates.Status.Set {
		fields = append(fields, "status = ?")
		args = append(args, nullableValue(updates.Status.Value))
	}
	if updates.Purpose.Set {
		fields = append(fields, "purpose = ?")
		args = append(args, nullableValue(updates.Purpose.Value))
	}
	if updates.Avatar.Set {
		fields = append(fields, "avatar = ?")
		args = append(args, nullableValue(updates.Avatar.Value))
	}
	if updates.LastSeen.Set {
		fields = append(fields, "last_seen = ?")
		args = append(args, nullableValue(updates.LastSeen.Value))
	}
	if updates.LeftAt.Set {
		fields = append(fields, "left_at = ?")
		args = append(args, nullableValue(updates.LeftAt.Value))
	}

	if len(fields) == 0 {
		return nil
	}

	args = append(args, agentID)
	query := fmt.Sprintf("UPDATE fray_agents SET %s WHERE agent_id = ?", strings.Join(fields, ", "))
	_, err := db.Exec(query, args...)
	return err
}

// GetActiveAgents returns non-stale agents.
func GetActiveAgents(db *sql.DB, staleHours int) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, avatar, registered_at, last_seen, left_at, managed, invoke, presence, mention_watermark, last_heartbeat, last_session_id
		FROM fray_agents
		WHERE left_at IS NULL
		  AND last_seen > (strftime('%s', 'now') - ? * 3600)
		ORDER BY last_seen DESC
	`, staleHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []types.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetAllAgents returns all agents.
func GetAllAgents(db *sql.DB) ([]types.Agent, error) {
	rows, err := db.Query(`
		SELECT guid, agent_id, status, purpose, avatar, registered_at, last_seen, left_at, managed, invoke, presence, mention_watermark, last_heartbeat, last_session_id
		FROM fray_agents
		ORDER BY agent_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []types.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetActiveUsers returns active chat users.
func GetActiveUsers(db *sql.DB) ([]string, error) {
	username, err := GetConfig(db, "username")
	if err != nil {
		return nil, err
	}
	if username == "" {
		return nil, nil
	}
	return []string{username}, nil
}

// GetAgentBases returns unique base agent IDs.
func GetAgentBases(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query(`SELECT agent_id FROM fray_agents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bases := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		lastDot := strings.LastIndex(id, ".")
		if lastDot != -1 {
			suffix := id[lastDot+1:]
			if isNumericSuffix(suffix) {
				bases[id[:lastDot]] = struct{}{}
				continue
			}
		}
		bases[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bases, nil
}

// IsAgentActive reports whether an agent is active.
func IsAgentActive(db *sql.DB, agentID string, staleHours int) (bool, error) {
	row := db.QueryRow(`
		SELECT 1 FROM fray_agents
		WHERE agent_id = ?
		  AND left_at IS NULL
		  AND last_seen > (strftime('%s', 'now') - ? * 3600)
	`, agentID, staleHours)
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

// RenameAgent updates IDs across tables.
func RenameAgent(db *sql.DB, oldID, newID string) error {
	oldAgent, err := GetAgent(db, oldID)
	if err != nil {
		return err
	}
	if oldAgent == nil {
		return fmt.Errorf("agent not found: %s", oldID)
	}

	existing, err := GetAgent(db, newID)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("agent already exists: %s", newID)
	}

	if _, err := db.Exec("UPDATE fray_agents SET agent_id = ? WHERE agent_id = ?", newID, oldID); err != nil {
		return err
	}
	if _, err := db.Exec("UPDATE fray_messages SET from_agent = ? WHERE from_agent = ?", newID, oldID); err != nil {
		return err
	}

	rows, err := db.Query(`
		SELECT guid, mentions FROM fray_messages
		WHERE mentions LIKE ?
	`, fmt.Sprintf("%%\"%s\"%%", oldID))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var guid string
		var mentionsJSON string
		if err := rows.Scan(&guid, &mentionsJSON); err != nil {
			return err
		}

		var mentions []string
		if err := json.Unmarshal([]byte(mentionsJSON), &mentions); err != nil {
			return err
		}

		updated := false
		for i, mention := range mentions {
			if mention == oldID {
				mentions[i] = newID
				updated = true
			}
		}
		if !updated {
			continue
		}
		updatedJSON, err := json.Marshal(mentions)
		if err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE fray_messages SET mentions = ? WHERE guid = ?", string(updatedJSON), guid); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if err := updateReactionsForAgent(db, oldID, newID); err != nil {
		return err
	}

	return UpdateClaimsAgentID(db, oldID, newID)
}

// MergeAgentHistory reassigns message history and claims from one agent to another.
func MergeAgentHistory(db *sql.DB, fromID, toID string) (int64, error) {
	if fromID == "" || toID == "" {
		return 0, fmt.Errorf("agent ids are required")
	}
	if fromID == toID {
		return 0, fmt.Errorf("cannot merge an agent into itself")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	updated, err := mergeAgentHistoryWith(tx, fromID, toID)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

func mergeAgentHistoryWith(tx *sql.Tx, fromID, toID string) (int64, error) {
	result, err := tx.Exec("UPDATE fray_messages SET from_agent = ? WHERE from_agent = ?", toID, fromID)
	if err != nil {
		return 0, err
	}
	messageCount, _ := result.RowsAffected()

	rows, err := tx.Query(`
		SELECT guid, mentions FROM fray_messages
		WHERE mentions LIKE ?
	`, fmt.Sprintf("%%\"%s\"%%", fromID))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var guid string
		var mentionsJSON string
		if err := rows.Scan(&guid, &mentionsJSON); err != nil {
			return 0, err
		}

		var mentions []string
		if err := json.Unmarshal([]byte(mentionsJSON), &mentions); err != nil {
			return 0, err
		}

		updated := false
		for i, mention := range mentions {
			if mention == fromID {
				mentions[i] = toID
				updated = true
			}
		}
		if !updated {
			continue
		}
		mentions = dedupeStrings(mentions)
		updatedJSON, err := json.Marshal(mentions)
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec("UPDATE fray_messages SET mentions = ? WHERE guid = ?", string(updatedJSON), guid); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if err := updateReactionsForAgent(tx, fromID, toID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec("UPDATE fray_claims SET agent_id = ? WHERE agent_id = ?", toID, fromID); err != nil {
		return 0, err
	}

	return messageCount, nil
}

func updateReactionsForAgent(db DBTX, oldID, newID string) error {
	// Update reactions in the new fray_reactions table
	if _, err := db.Exec(`
		UPDATE fray_reactions SET agent_id = ? WHERE agent_id = ?
	`, newID, oldID); err != nil {
		return err
	}

	// Also update legacy reactions JSON in messages table for backward compat
	rows, err := db.Query(`
		SELECT guid, reactions FROM fray_messages
		WHERE reactions LIKE ?
	`, fmt.Sprintf("%%\"%s\"%%", oldID))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var guid string
		var reactionsJSON string
		if err := rows.Scan(&guid, &reactionsJSON); err != nil {
			return err
		}

		var reactions map[string][]string
		if err := json.Unmarshal([]byte(reactionsJSON), &reactions); err != nil {
			return err
		}

		updated := false
		for reaction, users := range reactions {
			next := make([]string, 0, len(users))
			for _, user := range users {
				if user == oldID {
					user = newID
					updated = true
				}
				next = append(next, user)
			}
			reactions[reaction] = dedupeNonEmpty(next)
		}
		if !updated {
			continue
		}
		reactions = normalizeReactionsLegacy(reactions)
		updatedJSON, err := json.Marshal(reactions)
		if err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE fray_messages SET reactions = ? WHERE guid = ?", string(updatedJSON), guid); err != nil {
			return err
		}
	}
	return rows.Err()
}

// GetMaxVersion returns the highest version for a base name.
func GetMaxVersion(db *sql.DB, base string) (int, error) {
	pattern := fmt.Sprintf("%s.[0-9]*", base)
	rows, err := db.Query("SELECT agent_id FROM fray_agents WHERE agent_id GLOB ?", pattern)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	maxVersion := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		parts := strings.Split(id, ".")
		if len(parts) == 0 {
			continue
		}
		version := parseNumeric(parts[len(parts)-1])
		if version > maxVersion {
			maxVersion = version
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return maxVersion, nil
}

// NextVersion returns the next version for a base name.
func NextVersion(db *sql.DB, base string) (int, error) {
	max, err := GetMaxVersion(db, base)
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

func scanAgent(scanner interface{ Scan(dest ...any) error }) (types.Agent, error) {
	var row agentRow
	if err := scanner.Scan(&row.GUID, &row.AgentID, &row.Status, &row.Purpose, &row.Avatar, &row.RegisteredAt, &row.LastSeen, &row.LeftAt, &row.Managed, &row.Invoke, &row.Presence, &row.MentionWatermark, &row.LastHeartbeat, &row.LastSessionID); err != nil {
		return types.Agent{}, err
	}
	return row.toAgent(), nil
}

type agentRow struct {
	GUID             string
	AgentID          string
	Status           sql.NullString
	Purpose          sql.NullString
	Avatar           sql.NullString
	RegisteredAt     int64
	LastSeen         int64
	LeftAt           sql.NullInt64
	Managed          int
	Invoke           sql.NullString
	Presence         sql.NullString
	MentionWatermark sql.NullString
	LastHeartbeat    sql.NullInt64
	LastSessionID    sql.NullString
}

func (row agentRow) toAgent() types.Agent {
	agent := types.Agent{
		GUID:             row.GUID,
		AgentID:          row.AgentID,
		Status:           nullStringPtr(row.Status),
		Purpose:          nullStringPtr(row.Purpose),
		Avatar:           nullStringPtr(row.Avatar),
		RegisteredAt:     row.RegisteredAt,
		LastSeen:         row.LastSeen,
		LeftAt:           nullIntPtr(row.LeftAt),
		Managed:          row.Managed != 0,
		MentionWatermark: nullStringPtr(row.MentionWatermark),
		LastHeartbeat:    nullIntPtr(row.LastHeartbeat),
		LastSessionID:    nullStringPtr(row.LastSessionID),
	}
	if row.Presence.Valid {
		agent.Presence = types.PresenceState(row.Presence.String)
	}
	if row.Invoke.Valid && row.Invoke.String != "" {
		var invoke types.InvokeConfig
		if err := json.Unmarshal([]byte(row.Invoke.String), &invoke); err == nil {
			agent.Invoke = &invoke
		}
	}
	return agent
}
