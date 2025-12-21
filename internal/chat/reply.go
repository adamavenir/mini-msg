package chat

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

const (
	ReplyNone      = "none"
	ReplyResolved  = "resolved"
	ReplyAmbiguous = "ambiguous"
)

// ReplyMatch is a candidate reply target.
type ReplyMatch struct {
	GUID      string
	TS        int64
	FromAgent string
	Body      string
}

// ReplyResolution describes reply reference parsing.
type ReplyResolution struct {
	Kind    string
	Body    string
	ReplyTo string
	Prefix  string
	Match   *ReplyMatch
	Matches []ReplyMatch
}

var replyPrefixRe = regexp.MustCompile(`^\s*#([A-Za-z0-9-]{2,})\b`)

// ResolveReplyReference resolves a #prefix reply reference.
func ResolveReplyReference(db *sql.DB, text string) (ReplyResolution, error) {
	match := replyPrefixRe.FindStringSubmatchIndex(text)
	if match == nil {
		return ReplyResolution{Kind: ReplyNone, Body: text}, nil
	}

	prefix := normalizePrefix(text[match[2]:match[3]])
	if prefix == "" {
		return ReplyResolution{Kind: ReplyNone, Body: text}, nil
	}

	stripped := strings.TrimLeft(text[match[1]:], " \t")

	rows, err := db.Query(`
		SELECT guid, ts, from_agent, body
		FROM mm_messages
		WHERE guid LIKE ?
		ORDER BY ts DESC, guid DESC
		LIMIT 5
	`, fmt.Sprintf("msg-%s%%", prefix))
	if err != nil {
		return ReplyResolution{}, err
	}
	defer rows.Close()

	var matches []ReplyMatch
	for rows.Next() {
		var row ReplyMatch
		if err := rows.Scan(&row.GUID, &row.TS, &row.FromAgent, &row.Body); err != nil {
			return ReplyResolution{}, err
		}
		matches = append(matches, row)
	}
	if err := rows.Err(); err != nil {
		return ReplyResolution{}, err
	}

	if len(matches) == 0 {
		return ReplyResolution{Kind: ReplyNone, Body: text}, nil
	}
	if len(matches) == 1 {
		return ReplyResolution{
			Kind:    ReplyResolved,
			Body:    stripped,
			ReplyTo: matches[0].GUID,
			Match:   &matches[0],
		}, nil
	}

	return ReplyResolution{
		Kind:    ReplyAmbiguous,
		Body:    stripped,
		Prefix:  prefix,
		Matches: matches,
	}, nil
}

func normalizePrefix(raw string) string {
	if strings.HasPrefix(strings.ToLower(raw), "msg-") {
		return raw[4:]
	}
	return raw
}
