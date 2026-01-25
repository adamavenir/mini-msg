package chat

import (
	"strings"
	"unicode"
)

func (m *Model) refreshSuggestions() {
	value := m.input.Value()
	pos := m.inputCursorPos()
	if value == m.lastInputValue && pos == m.lastInputPos {
		return
	}
	m.lastInputValue = value
	m.lastInputPos = pos
	m.updateInputStyle()
	m.dismissHelpOnInput(value)

	// Check for slash command completion
	if strings.HasPrefix(value, "/") {
		spaceIdx := strings.Index(value, " ")
		if spaceIdx == -1 || pos <= spaceIdx {
			// Still typing the command name - show command suggestions
			prefix := value[1:pos]
			if spaceIdx != -1 && pos > spaceIdx {
				prefix = value[1:spaceIdx]
			}
			suggestions := buildCommandSuggestions(prefix)
			if len(suggestions) > 0 {
				m.suggestions = suggestions
				m.suggestionIndex = -1 // No selection until user navigates
				m.suggestionStart = 0
				m.suggestionKind = suggestionCommand
				m.resize()
				return
			}
		} else {
			// Command is complete (has space) - show dynamic suggestions or usage help
			cmdName := value[:spaceIdx]
			argPrefix := strings.TrimSpace(value[spaceIdx+1:])

			// Special handling for /run - show available scripts
			if cmdName == "/run" {
				scriptSuggestions := m.buildRunScriptSuggestions(argPrefix)
				if len(scriptSuggestions) > 0 {
					m.suggestions = scriptSuggestions
					m.suggestionIndex = -1
					m.suggestionStart = spaceIdx + 1
					m.suggestionKind = suggestionScript
					m.resize()
					return
				}
			}

			usageHelp := getCommandUsage(cmdName)
			if usageHelp != "" {
				m.suggestions = []suggestionItem{{
					Display: usageHelp,
					Insert:  "", // Not insertable
				}}
				m.suggestionIndex = -1 // No selection
				m.suggestionKind = suggestionCommand
				m.resize()
				return
			}
		}
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	kind, start, prefix := findSuggestionToken(value, pos)
	if kind == suggestionNone {
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	suggestions, err := m.buildSuggestions(kind, prefix)
	if err != nil {
		m.status = err.Error()
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}
	if len(suggestions) == 0 {
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	m.suggestions = suggestions
	m.suggestionIndex = -1 // No selection until user navigates
	m.suggestionStart = start
	m.suggestionKind = kind
	m.resize()
}

func findSuggestionToken(value string, cursor int) (suggestionKind, int, string) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	for i := cursor - 1; i >= 0; i-- {
		if unicode.IsSpace(runes[i]) {
			break
		}
		if runes[i] != '@' && runes[i] != '#' {
			continue
		}
		if i > 0 && isAlphaNum(runes[i-1]) {
			return suggestionNone, 0, ""
		}
		prefix := string(runes[i+1 : cursor])
		if runes[i] == '@' {
			return suggestionMention, i, prefix
		}
		normalized := normalizePrefix(prefix)
		if len(normalized) < 2 {
			return suggestionNone, 0, ""
		}
		return suggestionReply, i, normalized
	}
	return suggestionNone, 0, ""
}
