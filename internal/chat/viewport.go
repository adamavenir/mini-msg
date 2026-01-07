package chat

import (
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const pollInterval = time.Second

type pollMsg struct {
	roomMessages    []types.Message
	threadMessages  []types.Message
	threadID        string
	questions       []types.Question
	threads         []types.Thread
	mentionMessages []types.Message
}

func (m *Model) pollCmd() tea.Cmd {
	cursor := m.lastCursor
	mentionCursor := m.lastMentionCursor
	username := m.username
	includeArchived := m.includeArchived
	showUpdates := m.showUpdates
	currentThread := m.currentThread
	currentPseudo := m.currentPseudo

	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		options := types.MessageQueryOptions{Since: cursor, IncludeArchived: includeArchived}
		roomMessages, err := db.GetMessages(m.db, &options)
		if err != nil {
			return errMsg{err: err}
		}
		roomMessages, err = db.ApplyMessageEditCounts(m.projectDBPath, roomMessages)
		if err != nil {
			return errMsg{err: err}
		}
		roomMessages = filterUpdates(roomMessages, showUpdates)

		threadID := ""
		threadMessages := []types.Message(nil)
		if currentThread != nil {
			threadID = currentThread.GUID
			threadMessages, err = db.GetThreadMessages(m.db, currentThread.GUID)
			if err != nil {
				return errMsg{err: err}
			}
			threadMessages, err = db.ApplyMessageEditCounts(m.projectDBPath, threadMessages)
			if err != nil {
				return errMsg{err: err}
			}
			threadMessages = filterUpdates(threadMessages, showUpdates)
		}

		var questions []types.Question
		if currentPseudo != "" {
			roomOnly := true
			var threadGUID *string
			if currentThread != nil {
				roomOnly = false
				threadGUID = &currentThread.GUID
			}
			query := types.QuestionQueryOptions{
				ThreadGUID: threadGUID,
				RoomOnly:   roomOnly,
			}
			switch currentPseudo {
			case pseudoThreadOpen:
				query.Statuses = []types.QuestionStatus{types.QuestionStatusOpen}
			case pseudoThreadClosed:
				query.Statuses = []types.QuestionStatus{types.QuestionStatusAnswered}
			case pseudoThreadWonder:
				query.Statuses = []types.QuestionStatus{types.QuestionStatusUnasked}
			case pseudoThreadStale:
				query.Statuses = []types.QuestionStatus{types.QuestionStatusOpen}
			}
			questions, err = db.GetQuestions(m.db, &query)
			if err != nil {
				return errMsg{err: err}
			}
			if currentPseudo == pseudoThreadStale {
				cutoff := time.Now().Unix() - questionStaleSeconds
				filtered := make([]types.Question, 0, len(questions))
				for _, question := range questions {
					if question.CreatedAt > 0 && question.CreatedAt < cutoff {
						filtered = append(filtered, question)
					}
				}
				questions = filtered
			}
		}

		// Fetch thread list for live updates
		threads, err := db.GetThreads(m.db, &types.ThreadQueryOptions{})
		if err != nil {
			threads = nil // Don't fail the poll, just return empty
		}

		// Fetch mentions from all threads for notifications
		var mentionMessages []types.Message
		if username != "" {
			allHomes := "" // empty string = all homes (room + threads)
			mentionOpts := &types.MessageQueryOptions{
				Since:                  mentionCursor,
				IncludeArchived:        false,
				Home:                   &allHomes,
				IncludeRepliesToAgent:  username,
			}
			mentionMessages, _ = db.GetMessagesWithMention(m.db, username, mentionOpts)
		}

		return pollMsg{
			roomMessages:    roomMessages,
			threadMessages:  threadMessages,
			threadID:        threadID,
			questions:       questions,
			threads:         threads,
			mentionMessages: mentionMessages,
		}
	})
}

func (m *Model) refreshViewport(scrollToBottom bool) {
	content := m.renderMessages()
	m.viewport.SetContent(content)
	if scrollToBottom {
		m.viewport.GotoBottom()
		return
	}
	if m.viewport.Height <= 0 {
		return
	}
	maxOffset := lipgloss.Height(content) - m.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.viewport.YOffset > maxOffset {
		m.viewport.SetYOffset(maxOffset)
	}
}

func (m *Model) nearTop() bool {
	return m.viewport.YOffset <= 5
}

func (m *Model) loadOlderMessages() {
	if m.currentThread != nil || m.currentPseudo != "" {
		return
	}
	if !m.hasMore || m.oldestCursor == nil {
		return
	}

	options := &types.MessageQueryOptions{
		Before:          m.oldestCursor,
		Limit:           m.lastLimit,
		IncludeArchived: m.includeArchived,
	}

	prevHeight := lipgloss.Height(m.renderMessages())
	rawMessages, err := db.GetMessages(m.db, options)
	if err != nil {
		m.status = err.Error()
		return
	}
	if len(rawMessages) == 0 {
		m.hasMore = false
		return
	}

	first := rawMessages[0]
	m.oldestCursor = &types.MessageCursor{GUID: first.ID, TS: first.TS}
	if len(rawMessages) < m.lastLimit {
		m.hasMore = false
	}

	older := filterUpdates(rawMessages, m.showUpdates)
	if len(older) == 0 {
		return
	}

	m.messages = append(older, m.messages...)
	m.refreshViewport(false)
	newHeight := lipgloss.Height(m.renderMessages())
	delta := newHeight - prevHeight
	if delta > 0 {
		m.viewport.SetYOffset(m.viewport.YOffset + delta)
	}
}

func (m *Model) currentMessages() []types.Message {
	// For pseudo-threads (open-qs, etc), return source messages from questions
	if m.currentPseudo != "" {
		return m.questionSourceMessages()
	}
	var messages []types.Message
	if m.currentThread != nil {
		messages = m.threadMessages
	} else {
		messages = m.messages
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
