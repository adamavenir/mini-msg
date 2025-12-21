package core

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/types"
)

func parseRelativeTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	unit := value[len(value)-1:]
	amountStr := value[:len(value)-1]
	var multiplier int64
	switch strings.ToLower(unit) {
	case "m":
		multiplier = 60
	case "h":
		multiplier = 3600
	case "d":
		multiplier = 86400
	case "w":
		multiplier = 604800
	default:
		return nil
	}
	amount := int64(0)
	for _, r := range amountStr {
		if r < '0' || r > '9' {
			return nil
		}
		amount = amount*10 + int64(r-'0')
	}
	if amount == 0 {
		return nil
	}

	ts := time.Now().Add(-time.Duration(amount*multiplier) * time.Second)
	return &ts
}

func parseAbsoluteTime(value string) *time.Time {
	lower := strings.ToLower(strings.TrimSpace(value))
	now := time.Now()
	if lower == "today" {
		ts := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return &ts
	}
	if lower == "yesterday" {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		ts := today.Add(-24 * time.Hour)
		return &ts
	}
	return nil
}

func resolveGUIDCursor(db *sql.DB, expr string) (*types.MessageCursor, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return nil, nil
	}
	if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "msg-") {
		return nil, nil
	}

	raw := trimmed
	if strings.HasPrefix(raw, "#") {
		raw = raw[1:]
	}

	if strings.HasPrefix(raw, "msg-") {
		row := db.QueryRow("SELECT guid, ts FROM mm_messages WHERE guid = ?", raw)
		var guid string
		var ts int64
		if err := row.Scan(&guid, &ts); err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("message %s not found", raw)
			}
			return nil, err
		}
		return &types.MessageCursor{GUID: guid, TS: ts}, nil
	}

	if len(raw) < 2 {
		return nil, fmt.Errorf("GUID prefix too short: %s", raw)
	}

	like := fmt.Sprintf("msg-%s%%", raw)
	rows, err := db.Query(`
		SELECT guid, ts FROM mm_messages
		WHERE guid LIKE ?
		ORDER BY ts DESC, guid DESC
		LIMIT 5
	`, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type entry struct {
		GUID string
		TS   int64
	}
	var results []entry
	for rows.Next() {
		var row entry
		if err := rows.Scan(&row.GUID, &row.TS); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no message matches #%s", raw)
	}
	if len(results) > 1 {
		refs := make([]string, 0, len(results))
		for _, row := range results {
			reference := row.GUID
			if len(reference) >= 8 {
				reference = reference[4:8]
			}
			refs = append(refs, "#"+reference)
		}
		return nil, fmt.Errorf("ambiguous #%s. Matches: %s", raw, strings.Join(refs, ", "))
	}

	return &types.MessageCursor{GUID: results[0].GUID, TS: results[0].TS}, nil
}

func cursorForTime(ts time.Time, mode string) types.MessageCursor {
	guid := ""
	if mode == "since" {
		guid = "zzzzzzzz"
	}
	return types.MessageCursor{GUID: guid, TS: ts.Unix()}
}

// ParseTimeExpression converts a time expression or GUID into a cursor.
func ParseTimeExpression(db *sql.DB, expression string, mode string) (*types.MessageCursor, error) {
	trimmed := strings.TrimSpace(expression)

	guidCursor, err := resolveGUIDCursor(db, trimmed)
	if err != nil {
		return nil, err
	}
	if guidCursor != nil {
		return guidCursor, nil
	}

	if absolute := parseAbsoluteTime(trimmed); absolute != nil {
		cursor := cursorForTime(*absolute, mode)
		return &cursor, nil
	}

	if relative := parseRelativeTime(trimmed); relative != nil {
		cursor := cursorForTime(*relative, mode)
		return &cursor, nil
	}

	return nil, fmt.Errorf("invalid time expression: %s", expression)
}
