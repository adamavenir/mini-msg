package chat

import (
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/bubbles/textarea"
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

func (m *Model) inputCursorPos() int {
	value := m.input.Value()
	if value == "" {
		return 0
	}
	lines := strings.Split(value, "\n")
	row := m.input.Line()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
	}
	col := m.input.LineInfo().ColumnOffset
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
	_, reactionMode := reactionInputText(m.input.Value())
	if reactionMode == m.reactionMode {
		return
	}
	m.reactionMode = reactionMode
	if reactionMode {
		applyInputStyles(&m.input, reactionColor, reactionColor)
		return
	}
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
	input.FocusedStyle.Base = lipgloss.NewStyle().Foreground(textColor).Background(inputBg)
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(textColor).Background(inputBg)
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(caretColor).Background(inputBg)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(inputBg)
	input.BlurredStyle.Base = lipgloss.NewStyle().Foreground(blurColor).Background(inputBg)
	input.BlurredStyle.Text = lipgloss.NewStyle().Foreground(blurColor).Background(inputBg)
	input.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(caretColor).Background(inputBg)
	input.BlurredStyle.CursorLine = lipgloss.NewStyle().Background(inputBg)
}
