package chat

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handled, cmd := m.handleSuggestionKeys(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.handleThreadPanelKeys(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.handleSidebarKeys(msg); handled {
		return m, cmd
	}
	if msg.Type == tea.KeyRunes && !msg.Paste && msg.String() == "?" && m.input.Value() == "" {
		m.showHelp()
		return m, nil
	}
	if msg.Type == tea.KeyUp && m.input.Value() == "" {
		if m.prefillEditCommand() {
			return m, nil
		}
	}
	// Down key at end of text exits edit mode (mirrors Up entering it)
	if msg.Type == tea.KeyDown && m.editingMessageID != "" {
		if m.cursorAtEndOfInput() {
			m.exitEditMode()
			m.resize()
			return m, nil
		}
	}
	if msg.Type == tea.KeyCtrlJ {
		m.insertInputText("\n")
		return m, nil
	}
	if msg.Type == tea.KeyRunes && !msg.Paste && strings.ContainsRune(string(msg.Runes), '\n') {
		m.insertInputText(normalizeNewlines(string(msg.Runes)))
		return m, nil
	}
	if msg.Type == tea.KeyRunes && msg.Paste {
		pastedText := normalizeNewlines(string(msg.Runes))
		// Collapse multiline pastes into placeholder
		lines := strings.Split(pastedText, "\n")
		if len(lines) > 1 {
			// Replace with collapsed placeholder
			placeholder := fmt.Sprintf("[%d lines pasted]", len(lines))
			m.insertInputText(placeholder)
			// Store original pasted text for potential expansion
			m.pastedText = pastedText
		} else {
			m.insertInputText(pastedText)
		}
		return m, nil
	}
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.input.Value() != "" || m.replyToID != "" {
			m.input.Reset()
			m.clearSuggestions()
			// Clear reply reference without status message (silent clear)
			m.replyToID = ""
			m.replyToPreview = ""
			m.pastedText = "" // clear any stored paste
			m.lastInputValue = m.input.Value()
			m.lastInputPos = m.inputCursorPos()
			m.updateInputStyle()
			m.resize()
			return m, nil
		}
		return m, tea.Quit
	case tea.KeyEsc:
		if m.editingMessageID != "" {
			m.exitEditMode()
			m.resize()
			return m, nil
		}
		// Clear peek mode if active
		if m.isPeeking() {
			m.clearPeek()
			m.refreshViewport(false)
			return m, nil
		}
		// Clear reply reference if set
		if m.replyToID != "" {
			m.clearReplyTo()
			return m, nil
		}
	case tea.KeyBackspace, tea.KeyDelete:
		// Backspace at position 0 with reply set: clear reply (regardless of text content)
		if m.replyToID != "" && m.inputCursorPos() == 0 {
			m.clearReplyTo()
			return m, nil
		}
	case tea.KeyEnter:
		// Handle edit mode submission
		if m.editingMessageID != "" {
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				m.status = "Cannot save empty message"
				return m, nil
			}
			msgID := m.editingMessageID
			m.exitEditMode()
			m.resize()
			return m, m.submitEdit(msgID, value)
		}
		// DEBUG: log replyToID state at Enter press time
		debugLog(fmt.Sprintf("KeyEnter: replyToID=%q replyToPreview=%q", m.replyToID, m.replyToPreview))
		value := strings.TrimSpace(m.input.Value())
		// Expand collapsed paste placeholder before submission
		// The placeholder can be anywhere in the text, not just the whole message
		if m.pastedText != "" {
			// Find placeholder pattern like "[61 lines pasted]"
			placeholderPattern := regexp.MustCompile(`\[\d+ lines pasted\]`)
			if placeholderPattern.MatchString(value) {
				value = placeholderPattern.ReplaceAllString(value, m.pastedText)
			}
			m.pastedText = "" // clear stored paste
		}
		m.input.Reset()
		m.clearSuggestions()
		m.lastInputValue = m.input.Value()
		m.lastInputPos = m.inputCursorPos()
		m.updateInputStyle()
		m.resize()
		if value == "" {
			return m, nil
		}
		if handled, cmd := m.handleSlashCommand(value); handled {
			// Slash commands don't clear the new message notification
			return m, cmd
		}
		// Regular messages clear the new message notification and scroll to bottom
		m.clearNewMessageNotification()
		m.refreshViewport(true)
		return m, m.handleSubmit(value)
	case tea.KeyShiftTab:
		// Shift-Tab: open channels panel
		m.openChannelPanel()
		return m, nil
	case tea.KeyTab:
		// Tab: open threads panel
		if len(m.suggestions) > 0 {
			return m, nil
		}
		m.openThreadPanel()
		return m, nil
	case tea.KeyCtrlB:
		// Ctrl-B: toggle sidebar persistence
		m.toggleSidebarPersistence()
		return m, nil
	case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if (msg.Type == tea.KeyPgUp || msg.Type == tea.KeyHome) && m.nearTop() {
			m.loadOlderMessages()
		}
		// Clear new message notification if scrolled to bottom
		if m.atBottom() {
			m.clearNewMessageNotification()
		}
		return m, cmd
	}
	var cmd tea.Cmd
	// Allow input when: no panel focus OR peeking (and not in filter mode)
	// This lets you type while peeking without losing the peek context
	canType := !m.sidebarFocus && !m.threadPanelFocus
	canType = canType || (m.isPeeking() && !m.threadFilterActive)
	if canType {
		// Note: We no longer clear pastedText on typing - the placeholder
		// will be expanded with the original paste content on submit.
		// This allows users to type context around their paste.
		cmd = m.safeInputUpdate(msg)
		m.refreshSuggestions()
		m.resize()
	}
	return m, cmd
}
