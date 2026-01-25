package chat

import (
	"os"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// maybeNotify sends an OS notification if the message warrants it.
// Triggers on: direct @mention, reply to agent's message.
// Suppressed if: from self, in muted thread, event/surface message, user is not human.
func (m *Model) maybeNotify(msg types.Message) {
	// Only notify human users (not agents testing in chat)
	users, _ := db.GetActiveUsers(m.db)
	isHumanUser := false
	for _, u := range users {
		if u == m.username {
			isHumanUser = true
			break
		}
	}
	if !isHumanUser {
		return
	}

	// Skip messages from self
	if msg.FromAgent == m.username {
		return
	}

	// Skip event and surface messages
	if msg.Type == types.MessageTypeEvent || msg.Type == types.MessageTypeSurface {
		return
	}

	// Skip messages in muted threads
	if msg.Home != "" && m.mutedThreads[msg.Home] {
		return
	}

	// Check if should notify: direct mention or reply to own message
	shouldNotify := IsDirectMention(msg.Body, m.username) || IsReplyToAgent(m.db, msg, m.username)
	if !shouldNotify {
		return
	}

	_ = SendNotification(msg, m.projectName)
}

// checkGotoFile checks for a navigation request from notification click.
// The file format is: thread_guid#message_id or just message_id
func (m *Model) checkGotoFile() {
	data, err := os.ReadFile(GotoFilePath)
	if err != nil {
		return // File doesn't exist, nothing to do
	}

	// Remove the file immediately to avoid re-processing
	_ = os.Remove(GotoFilePath)

	target := strings.TrimSpace(string(data))
	if target == "" {
		return
	}

	// Parse target: thread_guid#message_id or just message_id
	var threadGUID, messageID string
	if idx := strings.Index(target, "#"); idx != -1 {
		threadGUID = target[:idx]
		messageID = target[idx+1:]
	} else {
		messageID = target
	}

	// Navigate to thread if specified
	if threadGUID != "" {
		thread, err := db.GetThread(m.db, threadGUID)
		if err == nil && thread != nil {
			m.currentThread = thread
			m.currentPseudo = ""
			m.threadMessages, _ = db.GetThreadMessages(m.db, threadGUID)
			m.markThreadAsRead(threadGUID)
			m.refreshViewport(true)
			m.status = "Navigated to thread from notification"
		}
	}

	// TODO: scroll to specific message if messageID is provided
	_ = messageID
}
