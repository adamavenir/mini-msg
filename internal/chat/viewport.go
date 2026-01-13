package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/adamavenir/fray/internal/usage"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tokenCache caches usage results to avoid repeated file parsing.
var tokenCache = struct {
	sync.RWMutex
	data map[string]tokenCacheEntry
}{data: make(map[string]tokenCacheEntry)}

type tokenCacheEntry struct {
	usage     *TokenUsage
	fetchedAt time.Time
}

const tokenCacheTTL = 30 * time.Second

// getTokenUsage fetches token usage for a session ID using internal/usage package.
// Returns nil if session not found or no usage data.
func getTokenUsage(sessionID string) *TokenUsage {
	if sessionID == "" {
		return nil
	}

	// Check cache
	tokenCache.RLock()
	if entry, ok := tokenCache.data[sessionID]; ok {
		if time.Since(entry.fetchedAt) < tokenCacheTTL {
			tokenCache.RUnlock()
			return entry.usage
		}
	}
	tokenCache.RUnlock()

	// Use internal/usage package to get session usage
	sessionUsage, err := usage.GetSessionUsage(sessionID)
	if err != nil || sessionUsage == nil || sessionUsage.InputTokens == 0 {
		// Cache the miss
		tokenCache.Lock()
		tokenCache.data[sessionID] = tokenCacheEntry{usage: nil, fetchedAt: time.Now()}
		tokenCache.Unlock()
		return nil
	}

	// Convert to TokenUsage format expected by panels
	tuiUsage := &TokenUsage{
		SessionID:   sessionUsage.SessionID,
		TotalTokens: sessionUsage.InputTokens + sessionUsage.OutputTokens,
		Entries: []TokenUsageEntry{
			{
				InputTokens:     sessionUsage.InputTokens,
				OutputTokens:    sessionUsage.OutputTokens,
				CacheReadTokens: sessionUsage.CachedTokens,
			},
		},
	}

	// Cache the result
	tokenCache.Lock()
	tokenCache.data[sessionID] = tokenCacheEntry{usage: tuiUsage, fetchedAt: time.Now()}
	tokenCache.Unlock()

	return tuiUsage
}

// daemonLockInfo matches the daemon.lock file format.
type daemonLockInfo struct {
	PID       int   `json:"pid"`
	StartedAt int64 `json:"started_at"`
}

// readDaemonStartedAt reads the daemon's started_at timestamp from daemon.lock.
// Returns 0 if the file doesn't exist or can't be read.
func readDaemonStartedAt(projectDBPath string) int64 {
	frayDir := filepath.Dir(projectDBPath)
	lockPath := filepath.Join(frayDir, "daemon.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0
	}
	var info daemonLockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return 0
	}
	return info.StartedAt
}

const pollInterval = time.Second
const activityPollInterval = 250 * time.Millisecond

type pollMsg struct {
	roomMessages    []types.Message
	threadMessages  []types.Message
	threadID        string
	questions       []types.Question
	threads         []types.Thread
	mentionMessages []types.Message
	managedAgents   []types.Agent
	agentTokenUsage map[string]*TokenUsage
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

		// Fetch managed agents for activity panel
		managedAgents, _ := db.GetManagedAgents(m.db)

		// Fetch token usage for active agents
		agentTokenUsage := make(map[string]*TokenUsage)
		for _, agent := range managedAgents {
			if agent.LastSessionID != nil && *agent.LastSessionID != "" {
				if usage := getTokenUsage(*agent.LastSessionID); usage != nil {
					agentTokenUsage[agent.AgentID] = usage
				}
			}
		}

		return pollMsg{
			roomMessages:    roomMessages,
			threadMessages:  threadMessages,
			threadID:        threadID,
			questions:       questions,
			threads:         threads,
			mentionMessages: mentionMessages,
			managedAgents:   managedAgents,
			agentTokenUsage: agentTokenUsage,
		}
	})
}

// activityPollMsg is a faster-polling message for activity panel updates only.
// This runs at 250ms to catch fast state transitions (spawning→prompting→prompted).
type activityPollMsg struct {
	managedAgents    []types.Agent
	agentTokenUsage  map[string]*TokenUsage
	daemonStartedAt  int64
}

func (m *Model) activityPollCmd() tea.Cmd {
	projectDBPath := m.projectDBPath
	return tea.Tick(activityPollInterval, func(time.Time) tea.Msg {
		// Fetch managed agents for activity panel
		managedAgents, _ := db.GetManagedAgents(m.db)

		// Fetch token usage for active agents
		agentTokenUsage := make(map[string]*TokenUsage)
		for _, agent := range managedAgents {
			if agent.LastSessionID != nil && *agent.LastSessionID != "" {
				if usage := getTokenUsage(*agent.LastSessionID); usage != nil {
					agentTokenUsage[agent.AgentID] = usage
				}
			}
		}

		// Check daemon started_at for restart detection
		daemonStartedAt := readDaemonStartedAt(projectDBPath)

		return activityPollMsg{
			managedAgents:    managedAgents,
			agentTokenUsage:  agentTokenUsage,
			daemonStartedAt:  daemonStartedAt,
		}
	})
}

func (m *Model) refreshViewport(scrollToBottom bool) {
	content := m.renderMessages()
	m.viewport.SetContent(content)
	if scrollToBottom {
		m.viewport.GotoBottom()
		m.clearNewMessageNotification() // Clear notification when scrolling to bottom
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

// atBottom returns true if the viewport is scrolled to (or near) the bottom
func (m *Model) atBottom() bool {
	if m.viewport.Height <= 0 {
		return true
	}
	content := m.viewport.View()
	contentHeight := lipgloss.Height(content)
	maxOffset := contentHeight - m.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	// Consider "at bottom" if within 3 lines of the bottom
	return m.viewport.YOffset >= maxOffset-3
}

// addNewMessageAuthor adds an author to the pending new message notification list
func (m *Model) addNewMessageAuthor(author string) {
	for _, existing := range m.newMessageAuthors {
		if existing == author {
			return // already tracked
		}
	}
	m.newMessageAuthors = append(m.newMessageAuthors, author)
}

// clearNewMessageNotification clears the new message bar
func (m *Model) clearNewMessageNotification() {
	m.newMessageAuthors = nil
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

// isPeeking returns true if we're in peek mode (viewing content without changing posting context)
func (m *Model) isPeeking() bool {
	return m.peekSource != peekSourceNone
}

// clearPeek exits peek mode
func (m *Model) clearPeek() {
	m.peekThread = nil
	m.peekPseudo = ""
	m.peekSource = peekSourceNone
}

// commitPeek switches the posting context to match what's being peeked, then exits peek mode
func (m *Model) commitPeek() {
	// Switch to the peeked thread/pseudo
	m.currentThread = m.peekThread
	m.currentPseudo = m.peekPseudo

	// Load thread messages if switching to a thread
	if m.currentThread != nil {
		m.threadMessages, _ = db.GetThreadMessages(m.db, m.currentThread.GUID)
		m.addRecentThread(*m.currentThread)
	} else {
		m.threadMessages = nil
	}

	// Clear peek state
	m.clearPeek()

	// Refresh view and recalculate layout
	m.resize()
	m.refreshViewport(true)
}

// displayThread returns the thread whose content should be displayed
// (either the peeked thread or the current thread)
func (m *Model) displayThread() *types.Thread {
	if m.isPeeking() {
		return m.peekThread // nil means peeking main room
	}
	return m.currentThread
}

// displayPseudo returns the pseudo-thread whose content should be displayed
// (either the peeked pseudo or the current pseudo)
func (m *Model) displayPseudo() pseudoThreadKind {
	if m.isPeeking() {
		return m.peekPseudo // "" means not viewing a pseudo-thread
	}
	return m.currentPseudo
}
