package chat

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) clearSuggestions() {
	m.suggestions = nil
	m.suggestionIndex = -1
	m.suggestionStart = 0
	m.suggestionKind = suggestionNone
}

func (m *Model) suggestionHeight() int {
	if len(m.suggestions) == 0 {
		return 0
	}
	return lipgloss.Height(m.renderSuggestions())
}

func (m *Model) renderSuggestions() string {
	if len(m.suggestions) == 0 {
		return ""
	}
	normalStyle := lipgloss.NewStyle().Foreground(metaColor)
	selectedStyle := lipgloss.NewStyle().Foreground(userColor).Bold(true)

	lines := make([]string, 0, len(m.suggestions))
	for i, suggestion := range m.suggestions {
		prefix := "  "
		style := normalStyle
		if i == m.suggestionIndex {
			prefix = "> "
			style = selectedStyle
		}
		line := prefix + suggestion.Display
		if m.mainWidth() > 0 {
			line = truncateLine(line, m.mainWidth())
		}
		lines = append(lines, style.Render(line))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) applySuggestion(item suggestionItem) {
	value := []rune(m.input.Value())
	cursor := m.inputCursorPos()
	start := m.suggestionStart
	if start < 0 || start > len(value) {
		start = cursor
	}
	if cursor < start {
		cursor = start
	}

	// For command suggestions, replace up to first space (or cursor if typing args)
	if m.suggestionKind == suggestionCommand {
		valueStr := m.input.Value()
		spaceIdx := strings.Index(valueStr, " ")
		if spaceIdx == -1 {
			// No space - replace entire input with command + space
			m.input.SetValue(item.Insert + " ")
		} else {
			// Space exists - replace only up to space, keep args
			m.input.SetValue(item.Insert + valueStr[spaceIdx:])
		}
		m.input.CursorEnd()
		m.clearSuggestions()
		m.lastInputValue = m.input.Value()
		m.lastInputPos = m.inputCursorPos()
		m.resize()
		return
	}

	// For script suggestions (after /run), replace from suggestionStart to cursor
	if m.suggestionKind == suggestionScript {
		valueStr := m.input.Value()
		before := valueStr[:start]
		m.input.SetValue(before + item.Insert + " ")
		m.input.CursorEnd()
		m.clearSuggestions()
		m.lastInputValue = m.input.Value()
		m.lastInputPos = m.inputCursorPos()
		m.resize()
		return
	}

	before := value[:start]
	after := value[cursor:]
	insertRunes := []rune(item.Insert)
	if len(after) == 0 {
		insertRunes = append(insertRunes, ' ')
	}

	updated := append(append(before, insertRunes...), after...)
	m.input.SetValue(string(updated))
	m.input.CursorEnd()
	m.clearSuggestions()
	m.lastInputValue = m.input.Value()
	m.lastInputPos = m.inputCursorPos()
	m.resize()
}
