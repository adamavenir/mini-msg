package chat

import (
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func (m *Model) refreshReactions() error {
	ids := make([]string, 0, len(m.messages))
	for _, msg := range m.messages {
		if msg.Type == types.MessageTypeEvent {
			continue
		}
		if !strings.HasPrefix(msg.ID, "msg-") {
			continue
		}
		ids = append(ids, msg.ID)
	}
	if len(ids) == 0 {
		return nil
	}

	updated, err := db.GetMessageReactionsNew(m.db, ids)
	if err != nil {
		return err
	}

	events := make([]types.Message, 0)
	for i, msg := range m.messages {
		if msg.Type == types.MessageTypeEvent {
			continue
		}
		next, ok := updated[msg.ID]
		if !ok {
			continue
		}
		added := diffReactions(msg.Reactions, next)
		if len(added) > 0 {
			for reaction, entries := range added {
				// Only show reaction events from other agents, not from current user
				agents := make([]string, 0, len(entries))
				for _, e := range entries {
					if e.AgentID != m.username {
						agents = append(agents, e.AgentID)
					}
				}
				if len(agents) > 0 {
					eventLine := core.FormatReactionEvent(agents, reaction, msg.ID, msg.Body)
					events = append(events, newEventMessage(eventLine))
				}
			}
		}
		if !reactionsEqual(msg.Reactions, next) {
			m.messages[i].Reactions = next
		}
	}

	if len(events) > 0 {
		m.messages = append(m.messages, events...)
		m.refreshViewport(true)
	}
	return nil
}

func (m *Model) ambiguousStatus(resolution ReplyResolution) string {
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	parts := make([]string, 0, len(resolution.Matches))
	for _, match := range resolution.Matches {
		prefix := core.GetGUIDPrefix(match.GUID, prefixLength)
		preview := truncatePreview(match.Body, 50)
		parts = append(parts, fmt.Sprintf("#%s (@%s) %s", prefix, match.FromAgent, preview))
	}
	return fmt.Sprintf("Ambiguous #%s: %s", resolution.Prefix, strings.Join(parts, " | "))
}
