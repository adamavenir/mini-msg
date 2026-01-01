package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
	"github.com/gobwas/glob"
	"modernc.org/sqlite"
)

const (
	sqliteConstraint       = 19
	sqliteConstraintUnique = 2067
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

// PruneExpiredClaims removes expired claims.
func PruneExpiredClaims(db *sql.DB) (int64, error) {
	now := time.Now().Unix()
	result, err := db.Exec("DELETE FROM fray_claims WHERE expires_at IS NOT NULL AND expires_at < ?", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CreateClaim inserts a new claim.
func CreateClaim(db *sql.DB, claim types.ClaimInput) (*types.Claim, error) {
	now := time.Now().Unix()
	result, err := db.Exec(`
		INSERT INTO fray_claims (agent_id, claim_type, pattern, reason, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, claim.AgentID, claim.ClaimType, claim.Pattern, claim.Reason, now, claim.ExpiresAt)
	if err != nil {
		if isConstraintError(err) {
			existing, lookupErr := GetClaim(db, claim.ClaimType, claim.Pattern)
			if lookupErr == nil && existing != nil {
				return nil, fmt.Errorf("already claimed by @%s: %s:%s", existing.AgentID, claim.ClaimType, claim.Pattern)
			}
		}
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &types.Claim{
		ID:        id,
		AgentID:   claim.AgentID,
		ClaimType: claim.ClaimType,
		Pattern:   claim.Pattern,
		Reason:    claim.Reason,
		CreatedAt: now,
		ExpiresAt: claim.ExpiresAt,
	}, nil
}

// GetClaim returns a claim by type and pattern.
func GetClaim(db *sql.DB, claimType types.ClaimType, pattern string) (*types.Claim, error) {
	row := db.QueryRow(`
		SELECT id, agent_id, claim_type, pattern, reason, created_at, expires_at
		FROM fray_claims
		WHERE claim_type = ? AND pattern = ?
	`, claimType, pattern)
	claim, err := scanClaim(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &claim, nil
}

// GetClaimsByAgent returns claims for an agent.
func GetClaimsByAgent(db *sql.DB, agentID string) ([]types.Claim, error) {
	rows, err := db.Query(`
		SELECT id, agent_id, claim_type, pattern, reason, created_at, expires_at
		FROM fray_claims
		WHERE agent_id = ?
		ORDER BY created_at
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanClaims(rows)
}

// GetClaimsByType returns claims of a type.
func GetClaimsByType(db *sql.DB, claimType types.ClaimType) ([]types.Claim, error) {
	rows, err := db.Query(`
		SELECT id, agent_id, claim_type, pattern, reason, created_at, expires_at
		FROM fray_claims
		WHERE claim_type = ?
		ORDER BY created_at
	`, claimType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanClaims(rows)
}

// GetAllClaims returns all active claims.
func GetAllClaims(db *sql.DB) ([]types.Claim, error) {
	if _, err := PruneExpiredClaims(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`
		SELECT id, agent_id, claim_type, pattern, reason, created_at, expires_at
		FROM fray_claims
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanClaims(rows)
}

// DeleteClaim removes a specific claim.
func DeleteClaim(db *sql.DB, claimType types.ClaimType, pattern string) (bool, error) {
	result, err := db.Exec("DELETE FROM fray_claims WHERE claim_type = ? AND pattern = ?", claimType, pattern)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteClaimsByAgent removes all claims for an agent.
func DeleteClaimsByAgent(db *sql.DB, agentID string) (int64, error) {
	result, err := db.Exec("DELETE FROM fray_claims WHERE agent_id = ?", agentID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// FindConflictingFileClaims returns conflicting file claims.
func FindConflictingFileClaims(db *sql.DB, filePaths []string, excludeAgent string) ([]types.Claim, error) {
	claims, err := GetClaimsByType(db, types.ClaimTypeFile)
	if err != nil {
		return nil, err
	}

	var conflicts []types.Claim
	for _, claim := range claims {
		if excludeAgent != "" && claim.AgentID == excludeAgent {
			continue
		}
		matcher, err := glob.Compile(claim.Pattern)
		if err != nil {
			continue
		}
		for _, filePath := range filePaths {
			if matcher.Match(filePath) {
				conflicts = append(conflicts, claim)
				break
			}
		}
	}

	return conflicts, nil
}

// CheckFileConflict returns the first conflicting file claim.
func CheckFileConflict(db *sql.DB, filePath, excludeAgent string) (*types.Claim, error) {
	conflicts, err := FindConflictingFileClaims(db, []string{filePath}, excludeAgent)
	if err != nil {
		return nil, err
	}
	if len(conflicts) == 0 {
		return nil, nil
	}
	return &conflicts[0], nil
}

// UpdateClaimsAgentID updates claim ownership.
func UpdateClaimsAgentID(db *sql.DB, oldID, newID string) error {
	_, err := db.Exec("UPDATE fray_claims SET agent_id = ? WHERE agent_id = ?", newID, oldID)
	return err
}

// GetClaimCountsByAgent returns counts grouped by agent.
func GetClaimCountsByAgent(db *sql.DB) (map[string]int64, error) {
	if _, err := PruneExpiredClaims(db); err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT agent_id, COUNT(*) as count
		FROM fray_claims
		GROUP BY agent_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var agentID string
		var count int64
		if err := rows.Scan(&agentID, &count); err != nil {
			return nil, err
		}
		counts[agentID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

func scanClaim(scanner interface{ Scan(dest ...any) error }) (types.Claim, error) {
	var row claimRow
	if err := scanner.Scan(&row.ID, &row.AgentID, &row.ClaimType, &row.Pattern, &row.Reason, &row.CreatedAt, &row.ExpiresAt); err != nil {
		return types.Claim{}, err
	}
	return row.toClaim(), nil
}

func scanClaims(rows *sql.Rows) ([]types.Claim, error) {
	var claims []types.Claim
	for rows.Next() {
		claim, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return claims, nil
}

func generateUniqueGUIDForTable(db *sql.DB, table, prefix string) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		guid, err := core.GenerateGUID(prefix)
		if err != nil {
			return "", err
		}
		row := db.QueryRow(fmt.Sprintf("SELECT 1 FROM %s WHERE guid = ?", table), guid)
		var exists int
		err = row.Scan(&exists)
		if err == sql.ErrNoRows {
			return guid, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to generate unique %s GUID", prefix)
}

func isNumericSuffix(s string) bool {
	return parseNumeric(s) > 0 && fmt.Sprintf("%d", parseNumeric(s)) == s
}

func parseNumeric(s string) int {
	value := 0
	if s == "" {
		return 0
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		value = value*10 + int(r-'0')
	}
	return value
}

func nullableValue[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}

func isConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()
		return code == sqliteConstraint || code == sqliteConstraintUnique
	}
	return false
}

type claimRow struct {
	ID        int64
	AgentID   string
	ClaimType types.ClaimType
	Pattern   string
	Reason    sql.NullString
	CreatedAt int64
	ExpiresAt sql.NullInt64
}

func (row claimRow) toClaim() types.Claim {
	return types.Claim{
		ID:        row.ID,
		AgentID:   row.AgentID,
		ClaimType: row.ClaimType,
		Pattern:   row.Pattern,
		Reason:    nullStringPtr(row.Reason),
		CreatedAt: row.CreatedAt,
		ExpiresAt: nullIntPtr(row.ExpiresAt),
	}
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func nullIntPtr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}
