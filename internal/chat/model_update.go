package chat

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSizeMsg(msg)
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.MouseMsg:
		return m.handleMouseMsg(msg)
	case pollMsg:
		return m.handlePollMsg(msg)
	case activityPollMsg:
		return m.handleActivityPollMsg(msg)
	case errMsg:
		return m.handleErrMsg(msg)
	case editResultMsg:
		return m.handleEditResultMsg(msg)
	case clickDebounceMsg:
		return m.handleClickDebounceMsg(msg)
	default:
		cmd := m.safeInputUpdate(msg)
		m.refreshSuggestions()
		return m, cmd
	}
}

func (m *Model) handleWindowSizeMsg(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.resize()
	return m, nil
}
