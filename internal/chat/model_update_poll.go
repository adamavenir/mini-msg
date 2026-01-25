package chat

import (
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handlePollMsg(msg pollMsg) (tea.Model, tea.Cmd) {
	// Build set of room message IDs to avoid double-notifying
	roomMsgIDs := make(map[string]struct{}, len(msg.roomMessages))
	for _, rm := range msg.roomMessages {
		roomMsgIDs[rm.ID] = struct{}{}
	}

	if len(msg.roomMessages) > 0 {
		incoming := m.filterNewMessages(msg.roomMessages)
		last := msg.roomMessages[len(msg.roomMessages)-1]
		m.lastCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}

		if len(incoming) > 0 {
			m.status = ""
			m.messages = append(m.messages, incoming...)
			for _, incomingMsg := range incoming {
				if incomingMsg.ArchivedAt == nil {
					m.messageCount++
				}
				m.maybeNotify(incomingMsg)
			}
			if m.currentThread == nil && m.currentPseudo == "" {
				// Check if user has scrolled up before deciding scroll behavior
				if m.atBottom() {
					m.refreshViewport(true)
					m.newMessageAuthors = nil // Clear any pending notifications
				} else {
					// User has scrolled up - track new message authors instead of scrolling
					m.refreshViewport(false)
					for _, incomingMsg := range incoming {
						m.addNewMessageAuthor(m.displayAgentLabel(incomingMsg))
					}
				}
				// Mark as read since user is viewing the room
				m.markRoomAsRead()
			}
		}
	}

	// Handle mention notifications from threads (not already in room messages)
	if len(msg.mentionMessages) > 0 {
		last := msg.mentionMessages[len(msg.mentionMessages)-1]
		m.lastMentionCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}
		// Persist watermark to avoid re-notifying on restart
		_ = db.SetReadTo(m.db, m.username, "mentions", last.ID, last.TS)

		for _, mentionMsg := range msg.mentionMessages {
			// Skip if already notified via room messages
			if _, inRoom := roomMsgIDs[mentionMsg.ID]; inRoom {
				continue
			}
			m.maybeNotify(mentionMsg)
		}
	}

	if msg.threadID != "" && m.currentThread != nil && m.currentThread.GUID == msg.threadID {
		prevCount := len(m.threadMessages)
		m.threadMessages = msg.threadMessages
		hasNewMessages := len(m.threadMessages) > prevCount
		if m.currentPseudo == "" {
			m.refreshViewport(hasNewMessages)
		}
		// Mark thread as read since user is viewing it
		if hasNewMessages {
			m.markThreadAsRead(msg.threadID)
		}
	}

	if msg.questions != nil && m.currentPseudo != "" {
		m.pseudoQuestions = msg.questions
		m.refreshViewport(true)
	}

	// Handle thread list updates (live updates, renames, deletions)
	if msg.threads != nil {
		// Check if current thread was deleted
		if m.currentThread != nil {
			threadStillExists := false
			for _, t := range msg.threads {
				if t.GUID == m.currentThread.GUID {
					threadStillExists = true
					// Update thread name if it changed
					if t.Name != m.currentThread.GUID {
						m.currentThread = &t
					}
					break
				}
			}
			// Auto-navigate away from deleted thread
			if !threadStillExists {
				m.currentThread = nil
				m.currentPseudo = ""
				m.threadMessages = nil
				m.refreshViewport(true)
				m.status = "Thread was deleted, returned to main"
			}
		}
		// Update thread list
		m.threads = msg.threads
	}

	// Update managed agents for activity panel
	if msg.managedAgents != nil {
		m.managedAgents = msg.managedAgents
	}

	// Update agent token usage
	if msg.agentTokenUsage != nil {
		m.agentTokenUsage = msg.agentTokenUsage
	}

	m.refreshQuestionCounts()
	m.refreshUnreadCounts()

	if err := m.refreshReactions(); err != nil {
		m.status = err.Error()
	}

	// Check for navigation request from notification click
	m.checkGotoFile()

	return m, m.pollCmd()
}
