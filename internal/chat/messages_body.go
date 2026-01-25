package chat

import (
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) markBodyZones(msgID string, body string, color lipgloss.Color) string {
	lines := strings.Split(body, "\n")
	textStyle := lipgloss.NewStyle().Foreground(color)
	idStyle := lipgloss.NewStyle().Foreground(color).Bold(true).Underline(true)
	styledLines := make([]string, len(lines))

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			styledLines[i] = line
			continue
		}

		// Find inline IDs and style them specially
		styledLine := m.styleLineWithInlineIDs(msgID, i, line, textStyle, idStyle)
		zoneID := fmt.Sprintf("line-%s-%d", msgID, i)
		styledLines[i] = m.zoneManager.Mark(zoneID, styledLine)
	}

	return strings.Join(styledLines, "\n")
}

// styleLineWithInlineIDs finds #-prefixed IDs in a line and styles them as bold+underline zones
func (m *Model) styleLineWithInlineIDs(msgID string, lineNum int, line string, textStyle, idStyle lipgloss.Style) string {
	matches := inlineIDPattern.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return textStyle.Render(line)
	}

	var result strings.Builder
	cursor := 0

	for idx, match := range matches {
		// Add text before this match
		if match[0] > cursor {
			result.WriteString(textStyle.Render(line[cursor:match[0]]))
		}

		// Style and zone the ID
		idText := line[match[0]:match[1]]
		styledID := idStyle.Render(idText)
		zoneID := fmt.Sprintf("inlineid-%s-%d-%d", msgID, lineNum, idx)
		result.WriteString(m.zoneManager.Mark(zoneID, styledID))

		cursor = match[1]
	}

	// Add remaining text after last match
	if cursor < len(line) {
		result.WriteString(textStyle.Render(line[cursor:]))
	}

	return result.String()
}

// formatTombstone renders a tombstone message with distinct styling.
// Tombstones indicate pruned message ranges and use a dimmed, bordered style.
func (m *Model) formatTombstone(msg types.Message, prefixLength int) string {
	width := m.mainWidth()

	// Style the body with inline ID highlighting
	idStyle := lipgloss.NewStyle().Foreground(metaColor).Bold(true).Underline(true)
	textStyle := lipgloss.NewStyle().Foreground(metaColor).Faint(true)

	// Find and style #msg-* IDs in the body
	body := m.styleTombstoneBody(msg.ID, msg.Body, textStyle, idStyle)

	if width > 0 {
		body = ansi.Wrap(body, width-4, "") // Account for border
	}

	// Add tombstone icon and GUID footer
	guidPrefix := core.GetGUIDPrefix(msg.ID, prefixLength)
	guidText := fmt.Sprintf("#%s", guidPrefix)
	guidPart := m.zoneManager.Mark("guid-"+msg.ID, textStyle.Render(guidText))

	// Build the tombstone with a subtle left border
	borderStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(metaColor).
		PaddingLeft(1).
		Foreground(metaColor).
		Faint(true)

	content := fmt.Sprintf("⏏︎ %s\n%s", body, guidPart)
	return borderStyle.Render(content)
}

// styleTombstoneBody finds #msg-* IDs in tombstone body and styles them as clickable zones
func (m *Model) styleTombstoneBody(msgID, body string, textStyle, idStyle lipgloss.Style) string {
	matches := inlineIDPattern.FindAllStringIndex(body, -1)
	if len(matches) == 0 {
		return textStyle.Render(body)
	}

	var result strings.Builder
	cursor := 0

	for idx, match := range matches {
		// Add text before this match
		if match[0] > cursor {
			result.WriteString(textStyle.Render(body[cursor:match[0]]))
		}

		// Style and zone the ID
		idText := body[match[0]:match[1]]
		styledID := idStyle.Render(idText)
		zoneID := fmt.Sprintf("tombstone-id-%s-%d", msgID, idx)
		result.WriteString(m.zoneManager.Mark(zoneID, styledID))

		cursor = match[1]
	}

	// Add remaining text after last match
	if cursor < len(body) {
		result.WriteString(textStyle.Render(body[cursor:]))
	}

	return result.String()
}
