package chat

import (
	"fmt"

	"github.com/adamavenir/fray/internal/core"
)

func (m *Model) buildReplySuggestions(prefix string) ([]suggestionItem, error) {
	normalized := normalizePrefix(prefix)
	if len(normalized) < 2 {
		return nil, nil
	}

	rows, err := m.db.Query(`
		SELECT guid, from_agent, body
		FROM fray_messages
		WHERE guid LIKE ?
		ORDER BY ts DESC, guid DESC
		LIMIT ?
	`, fmt.Sprintf("msg-%s%%", normalized), suggestionLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	suggestions := make([]suggestionItem, 0, suggestionLimit)
	for rows.Next() {
		var guid string
		var fromAgent string
		var body string
		if err := rows.Scan(&guid, &fromAgent, &body); err != nil {
			return nil, err
		}
		displayPrefix := core.GetGUIDPrefix(guid, prefixLength)
		preview := truncatePreview(body, 40)
		display := fmt.Sprintf("#%s @%s %s", displayPrefix, fromAgent, preview)
		suggestions = append(suggestions, suggestionItem{
			Display: display,
			Insert:  "#" + guid,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return suggestions, nil
}
