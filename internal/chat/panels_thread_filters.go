package chat

import (
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func (m *Model) startThreadFilter() {
	if !m.threadFilterActive {
		m.threadFilterActive = true
		m.threadFilter = ""
	}
	m.updateThreadMatches()
}

func (m *Model) resetThreadFilter() {
	m.threadFilterActive = false
	m.threadFilter = ""
	m.threadMatches = nil
	m.threadSearchResults = nil
	m.threadScrollOffset = 0
}

func (m *Model) updateThreadMatches() {
	if !m.threadFilterActive {
		m.threadMatches = nil
		m.threadSearchResults = nil
		return
	}

	term := strings.ToLower(strings.TrimSpace(m.threadFilter))

	// Search database for threads matching the filter (if we have a term)
	m.threadSearchResults = nil
	if term != "" && m.db != nil {
		allThreads, err := db.GetThreads(m.db, &types.ThreadQueryOptions{})
		if err == nil {
			// Build set of subscribed thread GUIDs
			subscribed := make(map[string]struct{})
			for _, t := range m.threads {
				subscribed[t.GUID] = struct{}{}
			}
			// Filter to matching threads not already subscribed
			for _, t := range allThreads {
				if _, ok := subscribed[t.GUID]; ok {
					continue
				}
				if strings.Contains(strings.ToLower(t.Name), term) {
					m.threadSearchResults = append(m.threadSearchResults, t)
				}
			}
		}
	}

	entries := m.threadEntries()
	matches := make([]int, 0, len(entries))
	for i, entry := range entries {
		if entry.Kind == threadEntrySeparator {
			continue
		}
		if term == "" || threadEntryMatchesFilter(entry, term) {
			matches = append(matches, i)
		}
	}
	m.threadMatches = matches
	if len(matches) > 0 {
		m.threadIndex = matches[0]
	}
}

func threadEntryMatchesFilter(entry threadEntry, term string) bool {
	label := strings.ToLower(entry.Label)
	return strings.Contains(label, term)
}
