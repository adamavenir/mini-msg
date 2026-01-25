package chat

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) handleSuggestionKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if len(m.suggestions) == 0 {
		return false, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.clearSuggestions()
		m.resize()
		return true, nil
	case tea.KeyUp:
		if m.suggestionIndex < 0 {
			// First navigation - start from bottom
			m.suggestionIndex = len(m.suggestions) - 1
		} else {
			m.suggestionIndex--
			if m.suggestionIndex < 0 {
				m.suggestionIndex = len(m.suggestions) - 1
			}
		}
		return true, nil
	case tea.KeyDown:
		if m.suggestionIndex < 0 {
			// First navigation - start from top
			m.suggestionIndex = 0
		} else {
			m.suggestionIndex++
			if m.suggestionIndex >= len(m.suggestions) {
				m.suggestionIndex = 0
			}
		}
		return true, nil
	case tea.KeyTab:
		// Tab selects first if none selected, then applies
		if m.suggestionIndex < 0 {
			m.suggestionIndex = 0
		}
		if m.suggestionIndex < len(m.suggestions) {
			m.applySuggestion(m.suggestions[m.suggestionIndex])
		}
		return true, nil
	case tea.KeyEnter:
		// Apply suggestion but don't consume Enter - let main handler submit
		if m.suggestionIndex >= 0 && m.suggestionIndex < len(m.suggestions) {
			m.applySuggestion(m.suggestions[m.suggestionIndex])
		}
		return false, nil
	}
	return false, nil
}
