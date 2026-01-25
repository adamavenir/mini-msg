package chat

import "github.com/adamavenir/fray/internal/db"

func (m *Model) refreshUnreadCounts() {
	if m.db == nil || m.username == "" {
		return
	}

	// Get thread GUIDs
	threadGUIDs := make([]string, 0, len(m.threads))
	for _, t := range m.threads {
		threadGUIDs = append(threadGUIDs, t.GUID)
	}

	// Get unread counts for threads
	counts, err := db.GetUnreadCountsForAgent(m.db, m.username, threadGUIDs)
	if err != nil {
		return
	}
	m.unreadCounts = counts

	// Get room unread count
	roomCount, err := db.GetRoomUnreadCount(m.db, m.username)
	if err != nil {
		return
	}
	m.roomUnreadCount = roomCount
}

func (m *Model) markRoomAsRead() {
	if m.db == nil || m.username == "" {
		return
	}
	// Get the latest room message
	if len(m.messages) == 0 {
		return
	}
	latest := m.messages[len(m.messages)-1]
	_ = db.SetReadTo(m.db, m.username, "", latest.ID, latest.TS)
}

func (m *Model) markThreadAsRead(threadGUID string) {
	if m.db == nil || m.username == "" {
		return
	}
	// Get the latest message in the thread
	messages, err := db.GetThreadMessages(m.db, threadGUID)
	if err != nil || len(messages) == 0 {
		return
	}
	latest := messages[len(messages)-1]
	_ = db.SetReadTo(m.db, m.username, threadGUID, latest.ID, latest.TS)
}
