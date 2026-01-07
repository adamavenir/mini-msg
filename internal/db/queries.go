package db

import (
	"database/sql"
	"fmt"

	"github.com/adamavenir/fray/internal/core"
)

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
