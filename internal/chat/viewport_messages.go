package chat

import (
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func (m *Model) currentMessages() []types.Message {
	// Use display thread/pseudo (which considers peek mode)
	displayThread := m.displayThread()
	displayPseudo := m.displayPseudo()

	// For pseudo-threads (open-qs, etc), return source messages from questions
	if displayPseudo != "" {
		return m.questionSourceMessages()
	}
	var messages []types.Message
	if displayThread != nil {
		// When peeking a different thread, fetch its messages directly
		if m.isPeeking() && (m.currentThread == nil || displayThread.GUID != m.currentThread.GUID) {
			peekMessages, err := db.GetThreadMessages(m.db, displayThread.GUID)
			if err == nil {
				peekMessages, _ = db.ApplyMessageEditCounts(m.projectDBPath, peekMessages)
				messages = filterUpdates(peekMessages, m.showUpdates)
			}
		} else {
			messages = m.threadMessages
		}
	} else {
		// Main room - filter out join/leave events to reduce clutter
		messages = filterJoinLeaveEvents(m.messages)
	}
	return filterDeletedMessages(messages)
}

func filterDeletedMessages(messages []types.Message) []types.Message {
	filtered := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.ArchivedAt != nil && msg.Body == "[deleted]" {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func (m *Model) filterNewMessages(incoming []types.Message) []types.Message {
	if len(incoming) == 0 {
		return nil
	}
	existing := make(map[string]struct{}, len(m.messages))
	for _, msg := range m.messages {
		existing[msg.ID] = struct{}{}
	}
	filtered := make([]types.Message, 0, len(incoming))
	for _, msg := range incoming {
		if _, ok := existing[msg.ID]; ok {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func (m *Model) refreshThreadMessages() {
	if m.currentThread == nil {
		m.threadMessages = nil
		return
	}
	messages, err := db.GetThreadMessages(m.db, m.currentThread.GUID)
	if err != nil {
		m.status = err.Error()
		return
	}
	messages, err = db.ApplyMessageEditCounts(m.projectDBPath, messages)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.threadMessages = filterUpdates(messages, m.showUpdates)
}
