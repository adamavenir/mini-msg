package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/types"
	"github.com/gobwas/glob"
	"modernc.org/sqlite"
)

const (
	sqliteConstraint       = 19
	sqliteConstraintUnique = 2067
)

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
