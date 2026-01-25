package chat

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleThreadPanelKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.threadPanelOpen {
		return false, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.threadFilterActive {
			m.resetThreadFilter()
			m.resize()
			return true, nil
		}
		// If viewing muted collection, exit to top level
		if m.viewingMutedCollection {
			m.viewingMutedCollection = false
			m.threadIndex = 0
			m.threadScrollOffset = 0
			return true, nil
		}
		// If drilled in, drill out instead of closing
		if m.drillDepth() > 0 {
			m.drillOutAction()
			return true, nil
		}
		// At top level - close panel and focus message pane
		m.closePanels()
		return true, nil
	}

	if !m.threadPanelFocus {
		return false, nil
	}

	if !m.threadFilterActive {
		if msg.Type == tea.KeySpace || (msg.Type == tea.KeyRunes && !msg.Paste && msg.String() == " ") {
			m.startThreadFilter()
			m.resize()
			return true, nil
		}
	}

	if m.threadFilterActive {
		switch msg.Type {
		case tea.KeyBackspace, tea.KeyCtrlH:
			if m.threadFilter != "" {
				runes := []rune(m.threadFilter)
				m.threadFilter = string(runes[:len(runes)-1])
			}
			m.updateThreadMatches()
			m.resize()
			return true, nil
		case tea.KeyEnter:
			if len(m.threadMatches) == 0 {
				return true, nil
			}
		case tea.KeyRunes:
			if msg.Paste || msg.String() == " " {
				return true, nil
			}
			m.threadFilter += string(msg.Runes)
			m.updateThreadMatches()
			m.resize()
			return true, nil
		}
	}

	switch msg.String() {
	case "j":
		m.moveThreadSelection(1)
		m.peekThreadEntry(peekSourceKeyboard)
		return true, nil
	case "k":
		m.moveThreadSelection(-1)
		m.peekThreadEntry(peekSourceKeyboard)
		return true, nil
	case "h":
		m.drillOutAction()
		return true, nil
	case "l":
		m.drillInAction()
		return true, nil
	case "f":
		// /f to toggle fave (when not in filter mode)
		if !m.threadFilterActive {
			m.toggleFaveSelectedThread()
			return true, nil
		}
	}

	switch msg.Type {
	case tea.KeyUp:
		m.moveThreadSelection(-1)
		m.peekThreadEntry(peekSourceKeyboard)
		return true, nil
	case tea.KeyDown:
		m.moveThreadSelection(1)
		m.peekThreadEntry(peekSourceKeyboard)
		return true, nil
	case tea.KeyLeft:
		m.drillOutAction()
		return true, nil
	case tea.KeyRight:
		m.drillInAction()
		return true, nil
	case tea.KeyEnter:
		// Select the current thread entry
		m.selectThreadEntry()
		m.resetThreadFilter()
		return true, nil
	case tea.KeyCtrlF:
		// Ctrl-f to toggle fave
		m.toggleFaveSelectedThread()
		return true, nil
	case tea.KeyCtrlN:
		// Ctrl-n to set nickname - store target and pre-fill input
		entries := m.threadEntries()
		if m.threadIndex >= 0 && m.threadIndex < len(entries) {
			entry := entries[m.threadIndex]
			if entry.Kind == threadEntryThread && entry.Thread != nil {
				m.pendingNicknameGUID = entry.Thread.GUID
				m.input.SetValue("/n ")
				m.input.CursorEnd()
				m.threadPanelFocus = false
				m.sidebarFocus = false
				m.status = fmt.Sprintf("Enter nickname for %s (or leave empty to clear)", entry.Thread.Name)
			}
		}
		return true, nil
	}

	return false, nil
}
