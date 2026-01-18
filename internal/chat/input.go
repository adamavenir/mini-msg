package chat

import (
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) insertInputText(text string) {
	if text == "" {
		return
	}
	m.input.InsertString(text)
	m.refreshSuggestions()
	m.resize()
}

// safeInputUpdate calls textarea.Update with panic recovery.
// The bubbles textarea can panic during cursor navigation at soft-wrap
// boundaries (known library issue). This wrapper catches such panics
// and preserves the current input state.
func (m *Model) safeInputUpdate(msg tea.Msg) tea.Cmd {
	defer func() {
		if r := recover(); r != nil {
			// Textarea panicked during update - preserve current state
			// This typically happens during word navigation (alt-arrow)
			// at soft-wrap boundaries in the bubbles library
			m.status = "Navigation error (recovered)"
		}
	}()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

func (m *Model) inputCursorPos() int {
	value := m.input.Value()
	if value == "" {
		return 0
	}
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		return 0
	}
	row := m.input.Line()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
	}
	// Safely get column offset - the textarea may return invalid values
	// during soft-wrap recalculation
	lineInfo := m.input.LineInfo()
	col := lineInfo.ColumnOffset
	if col < 0 {
		col = 0
	}
	lineRunes := []rune(lines[row])
	if col > len(lineRunes) {
		col = len(lineRunes)
	}

	pos := 0
	for i := 0; i < row; i++ {
		pos += len([]rune(lines[i])) + 1
	}
	pos += col

	total := len([]rune(value))
	if pos > total {
		pos = total
	}
	return pos
}

func (m *Model) updateInputStyle() {
	value := m.input.Value()
	_, reactionMode := reactionInputText(value)
	replyMode := !reactionMode && replyPrefixRe.MatchString(value)

	// Edit mode takes precedence - red text to indicate editing
	if m.editingMessageID != "" {
		m.reactionMode = false
		m.replyMode = false
		m.wasEditMode = true
		applyInputStyles(&m.input, editColor, editColor)
		return
	}

	// Skip style update if mode unchanged (optimization).
	// But always apply styles when exiting edit mode to reset from red.
	if !m.wasEditMode && reactionMode == m.reactionMode && replyMode == m.replyMode {
		return
	}

	m.wasEditMode = false
	m.reactionMode = reactionMode
	m.replyMode = replyMode

	if reactionMode {
		// Yellow text for reaction input
		applyInputStyles(&m.input, reactionColor, reactionColor)
		return
	}
	if replyMode {
		// Blue text for reply input
		applyInputStyles(&m.input, replyColor, replyColor)
		return
	}
	// Normal text
	applyInputStyles(&m.input, textColor, blurText)
}

func (m *Model) dismissHelpOnInput(value string) {
	if m.helpMessageID == "" || value == "" {
		return
	}
	if m.removeMessageByID(m.helpMessageID) {
		m.helpMessageID = ""
		m.refreshViewport(true)
	}
}

func (m *Model) prefillReply(msg types.Message) {
	prefix := msg.ID
	value := m.input.Value()
	match := replyPrefixRe.FindStringSubmatchIndex(value)
	if match != nil {
		rest := strings.TrimLeft(value[match[1]:], " \t")
		if rest == "" {
			value = fmt.Sprintf("#%s ", prefix)
		} else {
			value = fmt.Sprintf("#%s %s", prefix, rest)
		}
	} else if strings.TrimSpace(value) == "" {
		value = fmt.Sprintf("#%s ", prefix)
	} else {
		value = fmt.Sprintf("#%s %s", prefix, strings.TrimSpace(value))
	}
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.clearSuggestions()
	m.lastInputValue = m.input.Value()
	m.lastInputPos = m.inputCursorPos()
	m.dismissHelpOnInput(value)
	m.updateInputStyle()
	m.resize()
}

func normalizeNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

func reactionInputText(value string) (string, bool) {
	match := replyPrefixRe.FindStringSubmatchIndex(value)
	if match == nil {
		return "", false
	}
	stripped := strings.TrimLeft(value[match[1]:], " \t")
	return core.NormalizeReactionText(stripped)
}

func applyInputStyles(input *textarea.Model, textColor, blurColor lipgloss.Color) {
	applyInputStylesWithBg(input, textColor, blurColor, inputBg)
}

func applyInputStylesWithBg(input *textarea.Model, textColor, blurColor, bgColor lipgloss.Color) {
	input.FocusedStyle.Base = lipgloss.NewStyle().Foreground(textColor).Background(bgColor)
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(textColor).Background(bgColor)
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(caretColor).Background(bgColor)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(bgColor)
	input.BlurredStyle.Base = lipgloss.NewStyle().Foreground(blurColor).Background(bgColor)
	input.BlurredStyle.Text = lipgloss.NewStyle().Foreground(blurColor).Background(bgColor)
	input.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(caretColor).Background(bgColor)
	input.BlurredStyle.CursorLine = lipgloss.NewStyle().Background(bgColor)
}

// updateInputFocus blurs/focuses the input based on panel focus state.
// When a panel has focus, the input is blurred (dimmer, no cursor blink).
// Exception: when peeking, keep input focused so you can type while browsing.
func (m *Model) updateInputFocus() {
	panelHasFocus := m.threadPanelFocus || m.sidebarFocus
	// When peeking, keep input focused to allow typing while browsing
	if panelHasFocus && !m.isPeeking() {
		m.input.Blur()
	} else {
		m.input.Focus()
	}
}
