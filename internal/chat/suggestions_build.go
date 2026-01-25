package chat

import (
	"path/filepath"
	"strings"
)

func (m *Model) buildSuggestions(kind suggestionKind, prefix string) ([]suggestionItem, error) {
	switch kind {
	case suggestionMention:
		return m.buildMentionSuggestions(prefix)
	case suggestionReply:
		return m.buildReplySuggestions(prefix)
	default:
		return nil, nil
	}
}

// buildRunScriptSuggestions returns script suggestions for /run command.
// Searches both .fray/llm/run/ (fray scripts) and llm/run/ (project scripts).
func (m *Model) buildRunScriptSuggestions(prefix string) []suggestionItem {
	// Collect scripts from both locations
	frayRunDir := filepath.Join(m.projectRoot, ".fray", "llm", "run")
	projRunDir := filepath.Join(m.projectRoot, "llm", "run")

	seen := make(map[string]bool)
	suggestions := make([]suggestionItem, 0, suggestionLimit)
	prefix = strings.ToLower(prefix)

	// Add fray scripts first
	if scripts, err := listMlldScripts(frayRunDir); err == nil {
		for _, script := range scripts {
			if prefix != "" && !strings.HasPrefix(strings.ToLower(script), prefix) {
				continue
			}
			if seen[script] {
				continue
			}
			seen[script] = true
			suggestions = append(suggestions, suggestionItem{
				Display: script,
				Insert:  script,
			})
			if len(suggestions) >= suggestionLimit {
				return suggestions
			}
		}
	}

	// Add project scripts
	if scripts, err := listMlldScripts(projRunDir); err == nil {
		for _, script := range scripts {
			if prefix != "" && !strings.HasPrefix(strings.ToLower(script), prefix) {
				continue
			}
			if seen[script] {
				continue
			}
			seen[script] = true
			suggestions = append(suggestions, suggestionItem{
				Display: script,
				Insert:  script,
			})
			if len(suggestions) >= suggestionLimit {
				return suggestions
			}
		}
	}

	return suggestions
}
