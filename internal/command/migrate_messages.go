package command

import (
	"database/sql"
	"fmt"
	"strings"
)

type messageRow struct {
	GUID       *string
	ID         *int64
	TS         int64
	FromAgent  string
	Body       string
	Mentions   *string
	Type       *string
	ReplyTo    any
	EditedAt   *int64
	ArchivedAt *int64
}

func loadMessages(dbConn *sql.DB, hasGUID bool, hasID bool) ([]messageRow, error) {
	fields := []string{}
	if hasGUID {
		fields = append(fields, "guid")
	}
	if hasID {
		fields = append(fields, "id")
	}
	fields = append(fields, "ts", "from_agent", "body", "mentions", "type", "reply_to", "edited_at", "archived_at")
	query := fmt.Sprintf(`
		SELECT %s
		FROM fray_messages
		ORDER BY %s
	`, strings.Join(fields, ", "), messageOrder(hasID))

	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows, hasGUID, hasID)
}

func messageOrder(hasID bool) string {
	if hasID {
		return "ts ASC, id ASC"
	}
	return "ts ASC"
}

func scanMessages(rows *sql.Rows, hasGUID bool, hasID bool) ([]messageRow, error) {
	var messages []messageRow
	for rows.Next() {
		var (
			guid       sql.NullString
			id         sql.NullInt64
			ts         int64
			fromAgent  string
			body       string
			mentions   sql.NullString
			msgType    sql.NullString
			replyTo    any
			editedAt   sql.NullInt64
			archivedAt sql.NullInt64
		)

		switch {
		case hasGUID && hasID:
			if err := rows.Scan(&guid, &id, &ts, &fromAgent, &body, &mentions, &msgType, &replyTo, &editedAt, &archivedAt); err != nil {
				return nil, err
			}
		case hasGUID && !hasID:
			if err := rows.Scan(&guid, &ts, &fromAgent, &body, &mentions, &msgType, &replyTo, &editedAt, &archivedAt); err != nil {
				return nil, err
			}
		case !hasGUID && hasID:
			if err := rows.Scan(&id, &ts, &fromAgent, &body, &mentions, &msgType, &replyTo, &editedAt, &archivedAt); err != nil {
				return nil, err
			}
		default:
			if err := rows.Scan(&ts, &fromAgent, &body, &mentions, &msgType, &replyTo, &editedAt, &archivedAt); err != nil {
				return nil, err
			}
		}

		messages = append(messages, messageRow{
			GUID:       nullStringPtr(guid),
			ID:         nullInt64Ptr(id),
			TS:         ts,
			FromAgent:  fromAgent,
			Body:       body,
			Mentions:   nullStringPtr(mentions),
			Type:       nullStringPtr(msgType),
			ReplyTo:    replyTo,
			EditedAt:   nullInt64Ptr(editedAt),
			ArchivedAt: nullInt64Ptr(archivedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func parseMentions(raw *string) []string {
	if raw == nil || *raw == "" {
		return []string{}
	}
	parts := strings.Split(*raw, ",")
	var results []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}
