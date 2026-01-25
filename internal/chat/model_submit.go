package chat

import (
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleSubmit(text string) tea.Cmd {
	debugLog(fmt.Sprintf("handleSubmit called: text=%q, replyToID=%q", text, m.replyToID))
	body := text
	var replyTo *string
	var replyMatch *ReplyMatch

	// Use m.replyToID if set (from double-click reply), otherwise parse text for #msgid
	if m.replyToID != "" {
		debugLog(fmt.Sprintf("handleSubmit: using replyToID=%q", m.replyToID))
		// Copy the ID before clearing (don't take pointer to struct field we're about to modify)
		replyID := m.replyToID
		replyTo = &replyID
		// Clear the reply reference after copying
		m.replyToID = ""
		m.replyToPreview = ""
		m.resize() // Recalculate layout now that reply preview is gone
	} else {
		debugLog("handleSubmit: replyToID is empty, checking text for #msgid")
		// Check for inline #msgid reference in text
		resolution, err := ResolveReplyReference(m.db, text)
		if err != nil {
			m.status = err.Error()
			return nil
		}
		if resolution.Kind == ReplyAmbiguous {
			m.status = m.ambiguousStatus(resolution)
			return nil
		}
		if resolution.Kind == ReplyResolved {
			body = resolution.Body
			replyTo = &resolution.ReplyTo
			replyMatch = resolution.Match
		}
	}

	if replyTo != nil {
		if reaction, ok := core.NormalizeReactionText(body); ok {
			return m.handleReaction(reaction, *replyTo, replyMatch)
		}
	}

	if m.currentPseudo != "" {
		m.status = "Select a thread or #main to post"
		return nil
	}

	agentBases, err := db.GetAgentBases(m.db)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	mentionResult := core.ExtractMentionsWithSession(body, agentBases)
	mentions := core.ExpandAllMention(mentionResult.Mentions, agentBases)

	var replyMsg *types.Message
	if replyTo != nil && m.currentThread != nil {
		replyMsg, _ = db.GetMessage(m.db, *replyTo)
	}

	home := ""
	if m.currentThread != nil {
		home = m.currentThread.GUID
	}
	// Debug: log the exact replyTo value being used
	if replyTo != nil {
		debugLog(fmt.Sprintf("handleSubmit: creating message with replyTo=%q", *replyTo))
	} else {
		debugLog("handleSubmit: creating message with replyTo=nil")
	}
	created, err := db.CreateMessage(m.db, types.Message{
		FromAgent:    m.username,
		Body:         body,
		Mentions:     mentions,
		ForkSessions: mentionResult.ForkSessions,
		Type:         types.MessageTypeUser,
		ReplyTo:      replyTo,
		Home:         home,
	})
	if err != nil {
		m.status = err.Error()
		return nil
	}

	if err := db.AppendMessage(m.projectDBPath, created); err != nil {
		m.status = err.Error()
		return nil
	}

	if m.currentThread != nil {
		m.threadMessages = append(m.threadMessages, created)
	} else {
		m.messages = append(m.messages, created)
	}
	if m.currentThread == nil {
		m.lastCursor = &types.MessageCursor{GUID: created.ID, TS: created.TS}
	}
	if created.ArchivedAt == nil {
		m.messageCount++
	}
	m.status = ""
	if m.currentThread != nil && replyMsg != nil && replyMsg.Home != m.currentThread.GUID {
		if err := db.AddMessageToThread(m.db, m.currentThread.GUID, replyMsg.ID, m.username, time.Now().Unix()); err == nil {
			_ = db.AppendThreadMessage(m.projectDBPath, db.ThreadMessageJSONLRecord{
				ThreadGUID:  m.currentThread.GUID,
				MessageGUID: replyMsg.ID,
				AddedBy:     m.username,
				AddedAt:     time.Now().Unix(),
			})
		}
	}
	m.refreshViewport(true)

	// Mark as read immediately so our own message doesn't show as unread
	if m.currentThread != nil {
		m.markThreadAsRead(m.currentThread.GUID)
	} else {
		m.markRoomAsRead()
	}

	return nil
}

func (m *Model) handleReaction(reaction, messageID string, match *ReplyMatch) tea.Cmd {
	updated, reactedAt, err := db.AddReaction(m.db, messageID, m.username, reaction)
	if err != nil {
		m.status = err.Error()
		return nil
	}

	// Write reaction to JSONL (new format - separate record)
	if err := db.AppendReaction(m.projectDBPath, messageID, m.username, reaction, reactedAt); err != nil {
		m.status = err.Error()
		return nil
	}

	m.applyMessageUpdate(*updated)

	// Don't show event for user's own reactions - just update status briefly
	// This reduces clutter per fray-48xt; agent reaction events will still show via poll
	m.status = fmt.Sprintf("Reacted %s", reaction)
	m.refreshViewport(true)

	return nil
}
