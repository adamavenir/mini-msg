package chat

import (
	"fmt"

	"github.com/adamavenir/fray/internal/db"
)

// navigateToAgentThread navigates to the thread where the agent last posted.
func (m *Model) navigateToAgentThread(agentID string) {
	// Get the agent's last message
	lastMsg, err := db.GetLastMessageByAgent(m.db, agentID)
	if err != nil {
		m.status = fmt.Sprintf("Error finding %s's thread: %v", agentID, err)
		return
	}
	if lastMsg == nil {
		m.status = fmt.Sprintf("@%s has not posted yet", agentID)
		return
	}

	// Navigate based on home field
	home := lastMsg.Home
	if home == "" || home == "room" {
		// Agent's last post was in the room - go to room
		m.currentThread = nil
		m.currentPseudo = ""
		m.threadMessages = nil
		m.refreshViewport(true)
		m.status = fmt.Sprintf("@%s last posted in room", agentID)
		return
	}

	// Find the thread by GUID
	for _, thread := range m.threads {
		if thread.GUID == home {
			m.currentThread = &thread
			m.currentPseudo = ""
			m.threadMessages, _ = db.GetThreadMessages(m.db, thread.GUID)
			m.refreshViewport(true)
			m.status = fmt.Sprintf("@%s's last thread: %s", agentID, thread.Name)
			return
		}
	}

	// Thread not found in list (might be deleted/archived)
	m.status = fmt.Sprintf("@%s's last thread not found", agentID)
}

// navigateToThread navigates to a thread by GUID.
func (m *Model) navigateToThread(threadGUID string) {
	thread, err := db.GetThread(m.db, threadGUID)
	if err != nil || thread == nil {
		m.status = fmt.Sprintf("Thread not found: %s", threadGUID)
		return
	}

	m.currentThread = thread
	m.currentPseudo = ""
	m.threadMessages, _ = db.GetThreadMessages(m.db, thread.GUID)
	m.addRecentThread(*thread)
	m.refreshViewport(true)
	m.status = fmt.Sprintf("Thread: %s", thread.Name)
}
