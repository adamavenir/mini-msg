package chat

import (
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) copyFromZone(mouseMsg tea.MouseMsg, msg types.Message) {
	// Check which zone was clicked (GUID zone is handled separately for reply insertion)
	bylineZone := fmt.Sprintf("byline-%s", msg.ID)
	footerZone := fmt.Sprintf("footer-%s", msg.ID)

	var textToCopy string
	var description string

	// Check each zone type in priority order
	// Note: All text copies EXCLUDE speaker name per spec
	if m.zoneManager.Get(bylineZone).InBounds(mouseMsg) || m.zoneManager.Get(footerZone).InBounds(mouseMsg) {
		// Double-clicked on byline or footer - copy whole message body (no speaker)
		textToCopy = msg.Body
		description = "message"
	} else {
		// Check inline ID zones first (more specific than line zones)
		foundInlineID := false
		lines := strings.Split(msg.Body, "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			matches := inlineIDPattern.FindAllString(line, -1)
			for idx, idMatch := range matches {
				inlineIDZone := fmt.Sprintf("inlineid-%s-%d-%d", msg.ID, i, idx)
				if m.zoneManager.Get(inlineIDZone).InBounds(mouseMsg) {
					// Copy the ID without the # prefix
					textToCopy = idMatch[1:]
					description = "ID"
					foundInlineID = true
					break
				}
			}
			if foundInlineID {
				break
			}
		}

		if !foundInlineID {
			// Check line zones
			foundLine := false
			for i, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue // blank lines don't have zones
				}
				lineZone := fmt.Sprintf("line-%s-%d", msg.ID, i)
				if m.zoneManager.Get(lineZone).InBounds(mouseMsg) {
					textToCopy = line
					description = "line"
					foundLine = true
					break
				}
			}
			if !foundLine {
				// Fallback: copy whole message body (no speaker)
				textToCopy = msg.Body
				description = "message"
			}
		}
	}

	if err := copyToClipboard(textToCopy); err != nil {
		m.status = err.Error()
		return
	}
	m.status = fmt.Sprintf("Copied %s to clipboard.", description)
}

func (m *Model) copyMessage(msg types.Message) {
	// Copy message body without speaker name per spec
	if err := copyToClipboard(msg.Body); err != nil {
		m.status = err.Error()
		return
	}
	m.status = "Copied message to clipboard."
}
