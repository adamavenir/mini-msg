package chat

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Shift {
		return m, nil
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if handled, cmd := m.handleMouseClick(msg); handled {
			return m, cmd
		}
	}
	// Handle mouse wheel scrolling based on cursor position
	threadWidth := m.threadPanelWidth()
	sidebarWidth := m.sidebarWidth()
	isWheelUp := msg.Button == tea.MouseButtonWheelUp
	isWheelDown := msg.Button == tea.MouseButtonWheelDown
	if isWheelUp || isWheelDown {
		// Check if over thread panel
		if m.threadPanelOpen && msg.X < threadWidth {
			if isWheelUp {
				m.threadScrollOffset--
				if m.threadScrollOffset < 0 {
					m.threadScrollOffset = 0
				}
			} else {
				m.threadScrollOffset++
			}
			return m, nil
		}
		// Check if over sidebar
		if m.sidebarOpen && msg.X >= threadWidth && msg.X < threadWidth+sidebarWidth {
			if isWheelUp {
				m.sidebarScrollOffset--
				if m.sidebarScrollOffset < 0 {
					m.sidebarScrollOffset = 0
				}
			} else {
				m.sidebarScrollOffset++
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	if msg.Button == tea.MouseButtonWheelUp && m.nearTop() {
		m.loadOlderMessages()
	}
	// Clear new message notification if scrolled to bottom
	if m.atBottom() {
		m.clearNewMessageNotification()
	}
	return m, cmd
}
