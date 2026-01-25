package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) renderInput() string {
	var parts []string

	// Add new messages notification bar if user has scrolled up
	if newMsgBar := m.renderNewMessagesBar(); newMsgBar != "" {
		parts = append(parts, newMsgBar)
	}

	// Add reply preview if replying
	if replyPreview := m.renderReplyPreview(); replyPreview != "" {
		parts = append(parts, replyPreview)
	}

	content := m.input.View()
	style := lipgloss.NewStyle().Background(inputBg).Padding(0, inputPadding, 0, 0)
	if width := m.mainWidth(); width > 0 {
		style = style.Width(width)
	}
	blank := style.Render("")
	parts = append(parts, blank, style.Render(content), blank)
	return strings.Join(parts, "\n")
}

// renderReplyPreview renders the reply preview line above the input when replying
func (m *Model) renderReplyPreview() string {
	if m.replyToID == "" {
		return ""
	}

	// Style similar to reply context in messages
	previewStyle := lipgloss.NewStyle().Foreground(metaColor).Italic(true)
	cancelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	preview := previewStyle.Render(fmt.Sprintf("↪ Replying to: %s", m.replyToPreview))
	cancel := m.zoneManager.Mark("reply-cancel", cancelStyle.Render(" [x]"))

	width := m.mainWidth()
	if width > 0 {
		// Calculate padding to right-align the cancel button
		previewWidth := lipgloss.Width(preview)
		cancelWidth := lipgloss.Width(cancel)
		padding := width - previewWidth - cancelWidth
		if padding > 0 {
			return preview + strings.Repeat(" ", padding) + cancel
		}
	}
	return preview + " " + cancel
}

// renderNewMessagesBar renders a blue notification bar when new messages arrive while scrolled up
func (m *Model) renderNewMessagesBar() string {
	if len(m.newMessageAuthors) == 0 {
		return ""
	}

	// Build message text
	var text string
	if len(m.newMessageAuthors) == 1 {
		text = fmt.Sprintf("new message from @%s", m.newMessageAuthors[0])
	} else {
		mentions := make([]string, len(m.newMessageAuthors))
		for i, author := range m.newMessageAuthors {
			mentions[i] = "@" + author
		}
		text = fmt.Sprintf("new messages from %s", strings.Join(mentions, " "))
	}

	barStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")). // white text
		Background(lipgloss.Color("24")). // blue background (same as peek mode)
		Padding(0, 1)

	width := m.mainWidth()
	if width > 0 {
		barStyle = barStyle.Width(width)
	}

	return barStyle.Render(text)
}
