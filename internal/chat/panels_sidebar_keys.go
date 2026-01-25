package chat

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) handleSidebarKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.sidebarOpen {
		return false, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.sidebarFilterActive {
			m.resetSidebarFilter()
			m.resize()
			return true, nil
		}
		// Close panel and focus message pane
		m.closePanels()
		return true, nil
	}

	if !m.sidebarFocus {
		return false, nil
	}

	if !m.sidebarFilterActive {
		if msg.Type == tea.KeySpace || (msg.Type == tea.KeyRunes && !msg.Paste && (msg.String() == "#" || msg.String() == " ")) {
			m.startSidebarFilter()
			m.resize()
			return true, nil
		}
	}

	if m.sidebarFilterActive {
		switch msg.Type {
		case tea.KeyBackspace, tea.KeyCtrlH:
			if m.sidebarFilter != "" {
				runes := []rune(m.sidebarFilter)
				m.sidebarFilter = string(runes[:len(runes)-1])
			}
			m.updateSidebarMatches()
			m.resize()
			return true, nil
		case tea.KeyEnter:
			if len(m.sidebarMatches) == 0 {
				return true, nil
			}
		case tea.KeyRunes:
			if msg.Paste || msg.String() == " " {
				return true, nil
			}
			m.sidebarFilter += string(msg.Runes)
			m.updateSidebarMatches()
			m.resize()
			return true, nil
		}
	}

	switch msg.String() {
	case "j":
		m.moveChannelSelection(1)
		return true, nil
	case "k":
		m.moveChannelSelection(-1)
		return true, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		m.moveChannelSelection(-1)
		return true, nil
	case tea.KeyDown:
		m.moveChannelSelection(1)
		return true, nil
	case tea.KeyEnter:
		// Check if we want to select or just close and focus message pane
		if m.sidebarFilterActive {
			return true, m.selectChannelCmd()
		}
		// Close sidebar and focus message pane
		m.closePanels()
		return true, nil
	}

	return false, nil
}

func (m *Model) sidebarIndexAtLine(line int) int {
	// Account for top padding for pinned permissions
	pinnedHeight := m.pinnedPermissionsHeight()
	if line < pinnedHeight {
		return -1
	}
	line -= pinnedHeight

	// Sidebar has: header(1) + blank(1) = 2 lines before content
	headerLines := 2
	if line < headerLines {
		return -1
	}
	if len(m.channels) == 0 {
		return -1
	}
	indices := m.sidebarMatches
	if !m.sidebarFilterActive {
		indices = make([]int, len(m.channels))
		for i := range m.channels {
			indices[i] = i
		}
	}
	if len(indices) == 0 {
		return -1
	}
	// Convert Y coordinate to index: subtract headers, add scroll offset
	contentLine := line - headerLines
	actualIndex := contentLine + m.sidebarScrollOffset
	if actualIndex < 0 || actualIndex >= len(indices) {
		return -1
	}
	return indices[actualIndex]
}

func (m *Model) moveChannelSelection(delta int) {
	if len(m.channels) == 0 {
		return
	}
	if m.sidebarFilterActive {
		if len(m.sidebarMatches) == 0 {
			return
		}
		current := 0
		found := false
		for i, index := range m.sidebarMatches {
			if index == m.channelIndex {
				current = i
				found = true
				break
			}
		}
		if !found {
			m.channelIndex = m.sidebarMatches[0]
			return
		}
		next := current + delta
		if next < 0 {
			next = len(m.sidebarMatches) - 1
		} else if next >= len(m.sidebarMatches) {
			next = 0
		}
		m.channelIndex = m.sidebarMatches[next]
		return
	}

	index := m.channelIndex + delta
	if index < 0 {
		index = len(m.channels) - 1
	} else if index >= len(m.channels) {
		index = 0
	}
	m.channelIndex = index
}
