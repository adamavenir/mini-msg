package chat

import (
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) messageAtLine(line int) (*types.Message, bool) {
	if line < 0 {
		return nil, false
	}
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	cursor := 0
	messages := m.currentMessages()

	// Build read_to map (same as renderMessages to ensure consistent line heights)
	home := "room"
	if m.currentThread != nil {
		home = m.currentThread.GUID
	}
	readToMap := make(map[string][]string)
	if positions, err := db.GetReadToForHome(m.db, home); err == nil {
		for _, pos := range positions {
			readToMap[pos.MessageGUID] = append(readToMap[pos.MessageGUID], pos.AgentID)
		}
	}

	// Get anchor message GUID if viewing a thread (for subthread tree rendering)
	var anchorGUID string
	if m.currentThread != nil && m.currentThread.AnchorMessageGUID != nil {
		anchorGUID = *m.currentThread.AnchorMessageGUID
	}

	for i, msg := range messages {
		formatted := m.formatMessage(msg, prefixLength, readToMap)
		lines := lipgloss.Height(formatted)
		if line >= cursor && line < cursor+lines {
			if msg.Type == types.MessageTypeEvent {
				return nil, true
			}
			return &messages[i], true
		}
		cursor += lines

		// Account for subthread tree after anchor message (same as renderMessages)
		if anchorGUID != "" && msg.ID == anchorGUID {
			if tree := m.renderSubthreadTree(); tree != "" {
				treeLines := lipgloss.Height(tree)
				if line >= cursor && line < cursor+treeLines {
					return nil, true // clicked on tree, not a message
				}
				cursor += treeLines
			}
		}

		if i < len(messages)-1 {
			if line == cursor {
				return nil, true
			}
			cursor++
		}
	}
	return nil, false
}
