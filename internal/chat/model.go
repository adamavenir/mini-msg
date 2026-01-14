package chat

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

const doubleClickInterval = 400 * time.Millisecond
const singleClickDebounce = 300 * time.Millisecond // Wait this long before executing single-click

// debugLog writes debug messages to a file for debugging TUI issues
func debugLog(msg string) {
	f, err := os.OpenFile("/tmp/fray-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(time.Now().Format("15:04:05.000") + " " + msg + "\n")
}

// pendingClick stores info about a click waiting for possible double-click
type pendingClick struct {
	messageID string
	zone      string // "guid", "inlineid", or "line"
	text      string // text to copy if single-click executes
	timestamp time.Time
}

// peekSourceKind indicates how a peek was triggered (for displaying action hints)
type peekSourceKind int

const (
	peekSourceNone peekSourceKind = iota
	peekSourceKeyboard // j/k or arrow keys
	peekSourceClick    // mouse click
)

var (
	userColor     = lipgloss.Color("249")
	statusColor   = lipgloss.Color("241")
	metaColor     = lipgloss.Color("242")
	inputBg       = lipgloss.Color("236")
	editColor     = lipgloss.Color("203") // bright red text for edit mode
	peekBg        = lipgloss.Color("24")  // blue background for peek mode statusline
	caretColor    = lipgloss.Color("243")
	reactionColor = lipgloss.Color("220") // yellow for reaction input
	replyColor    = lipgloss.Color("75")  // blue for reply input
	textColor     = lipgloss.Color("255")
	blurText      = lipgloss.Color("248")
)

// Options configure chat.
type Options struct {
	DB              *sql.DB
	ProjectName     string
	ProjectRoot     string
	ProjectDBPath   string
	Username        string
	Last            int
	ShowUpdates     bool
	IncludeArchived bool
	DebugSync       bool
}

// Run starts the chat UI.
func Run(opts Options) error {
	model, err := NewModel(opts)
	if err != nil {
		return err
	}
	// Set window title (ANSI OSC sequence)
	title := "fray"
	if opts.ProjectName != "" {
		title = "fray · " + opts.ProjectName
	}
	fmt.Printf("\033]0;%s\007", title)

	// Write TTY path to temp file for notification focus script
	if ttyPath, err := os.Readlink("/dev/fd/0"); err == nil {
		_ = os.WriteFile("/tmp/fray-tty", []byte(ttyPath), 0644)
	}

	program := tea.NewProgram(model, tea.WithMouseCellMotion())
	_, err = program.Run()

	// Clean up TTY file on exit
	_ = os.Remove("/tmp/fray-tty")
	model.Close()
	return err
}

// Model implements the chat UI.
type Model struct {
	db                  *sql.DB
	projectName         string
	projectRoot         string
	projectDBPath       string
	username            string
	showUpdates         bool
	includeArchived     bool
	viewport            viewport.Model
	input               textarea.Model
	messages            []types.Message
	lastCursor          *types.MessageCursor
	lastMentionCursor   *types.MessageCursor
	oldestCursor        *types.MessageCursor
	status              string
	width               int
	height              int
	messageCount        int
	lastLimit           int
	hasMore             bool
	colorMap            map[string]lipgloss.Color
	suggestions         []suggestionItem
	suggestionIndex     int
	suggestionStart     int
	suggestionKind      suggestionKind
	reactionMode        bool
	replyMode           bool
	editingMessageID    string // non-empty when in edit mode
	lastInputValue      string
	lastInputPos        int
	threads                 []types.Thread
	threadIndex             int
	threadPanelOpen         bool
	threadPanelFocus        bool
	cachedThreadPanelWidth  int               // calculated width (snapshot when panel opens)
	threadFilter        string
	threadMatches       []int
	threadFilterActive  bool
	threadSearchResults []types.Thread
	recentThreads       []types.Thread
	visitedThreads      map[string]types.Thread
	currentThread       *types.Thread
	currentPseudo       pseudoThreadKind
	threadMessages      []types.Message
	questionCounts      map[pseudoThreadKind]int
	pseudoQuestions     []types.Question
	unreadCounts        map[string]int    // unread message counts per thread GUID
	roomUnreadCount     int               // unread count for main room
	collapsedThreads    map[string]bool   // collapsed state per thread GUID
	favedThreads        map[string]bool   // faved threads for current user
	subscribedThreads   map[string]bool   // subscribed threads for current user
	mutedThreads           map[string]bool   // muted threads for current user
	viewingMutedCollection bool              // true when drilled into muted collection view
	threadNicknames        map[string]string // thread nicknames for current user
	avatarMap              map[string]string // agent_id -> avatar character
	drillPath              []string          // current drill path (thread GUIDs from root to current)
	threadScrollOffset  int               // scroll offset for virtual scrolling in thread panel
	zoneManager         *zone.Manager     // bubblezone manager for click tracking
	channels            []channelEntry
	channelIndex        int
	sidebarOpen          bool
	sidebarFocus         bool
	sidebarFilter        string
	sidebarMatches       []int
	sidebarFilterActive  bool
	sidebarScrollOffset  int               // scroll offset for virtual scrolling in channel sidebar
	sidebarPersistent    bool              // if true, Tab just changes focus (doesn't close)
	pendingNicknameGUID  string            // thread GUID for pending /n command (set by Ctrl-N)
	helpMessageID       string
	initialScroll       bool
	lastClickID         string
	lastClickAt         time.Time
	// Activity panel state
	managedAgents         []types.Agent          // daemon-managed agents
	agentUnreadCounts     map[string]int         // unread count per agent
	agentTokenUsage       map[string]*TokenUsage // token usage per agent (by session ID)
	activityDrillOffline  bool                   // true when viewing offline agents only
	statusInvoker         *StatusInvoker         // mlld invoker for status display customization
	expandedJobClusters   map[string]bool        // job ID -> expanded (true = show workers)
	// Presence debounce state (suppress flicker from rapid presence changes)
	agentDisplayPresence map[string]types.PresenceState // presence currently being displayed
	agentActualPresence  map[string]types.PresenceState // actual presence from last poll
	agentPresenceChanged map[string]time.Time      // when actual presence last changed
	// Animation frame counter for spawn cycle (incremented every activity poll)
	animationFrame       int
	// New message notification state (when user has scrolled up)
	newMessageAuthors    []string // authors of messages received while scrolled up
	// Peek mode state (view thread without changing posting context)
	peekThread       *types.Thread // thread being peeked (nil if not peeking)
	peekPseudo       pseudoThreadKind // pseudo-thread being peeked (empty if not peeking)
	peekSource       peekSourceKind // how peek was triggered (for hint display)
	// Click debounce state (wait for possible double-click before executing single-click)
	pendingClick     *pendingClick // pending single-click waiting for timeout
	// Reply reference state (for reply preview UI)
	replyToID        string // message ID being replied to (empty if no reply)
	replyToPreview   string // preview text of reply target
	// Debug sync state
	debugSync        bool
	stopSyncChecker  chan struct{}
	// Daemon restart detection
	daemonStartedAt  int64
}

// TokenUsage holds token usage data from ccusage (for activity panel).
type TokenUsage struct {
	SessionID   string            `json:"sessionId"`
	TotalCost   float64           `json:"totalCost"`
	TotalTokens int64             `json:"totalTokens"`
	Entries     []TokenUsageEntry `json:"entries"`
}

// TokenUsageEntry represents a single API call from ccusage.
type TokenUsageEntry struct {
	Timestamp       string `json:"timestamp"`
	InputTokens     int64  `json:"inputTokens"`
	OutputTokens    int64  `json:"outputTokens"`
	CacheReadTokens int64  `json:"cacheReadTokens"`
}

// ContextTokens returns an estimate of current context window usage.
// Uses cacheReadTokens from the most recent entry as proxy for context size.
func (t *TokenUsage) ContextTokens() int64 {
	if t == nil || len(t.Entries) == 0 {
		return 0
	}
	// Most recent entry's cacheReadTokens approximates context size
	lastEntry := t.Entries[len(t.Entries)-1]
	return lastEntry.CacheReadTokens + lastEntry.InputTokens + lastEntry.OutputTokens
}

type errMsg struct {
	err error
}

// clickDebounceMsg fires after singleClickDebounce to execute pending single-click
type clickDebounceMsg struct {
	messageID string
	timestamp time.Time
}

// NewModel creates a chat model with recent messages loaded.
func NewModel(opts Options) (*Model, error) {
	if opts.Last <= 0 {
		opts.Last = 20
	}

	channels, channelIndex := loadChannels(opts.ProjectRoot)
	threads, threadIndex := loadThreads(opts.DB, opts.Username)

	// Load managed agents for activity panel (initial state before first poll)
	// Note: Token usage is deferred to background poll to avoid blocking startup
	managedAgents, _ := db.GetManagedAgents(opts.DB)
	agentTokenUsage := make(map[string]*TokenUsage)

	colorMap, err := buildColorMap(opts.DB, 50, opts.IncludeArchived)
	if err != nil {
		return nil, err
	}

	input := textarea.New()
	input.CharLimit = 0
	input.ShowLineNumbers = false
	input.MaxHeight = inputMaxHeight
	input.Cursor.SetChar("▍")
	input.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "› "
		}
		return "  "
	})
	applyInputStyles(&input, textColor, blurText)
	input.Focus()

	vp := viewport.New(0, 0)

	rawMessages, err := db.GetMessages(opts.DB, &types.MessageQueryOptions{
		Limit:           opts.Last,
		IncludeArchived: opts.IncludeArchived,
	})
	if err != nil {
		return nil, err
	}
	rawMessages, err = db.ApplyMessageEditCounts(opts.ProjectDBPath, rawMessages)
	if err != nil {
		return nil, err
	}
	messages := filterUpdates(rawMessages, opts.ShowUpdates)

	var lastCursor *types.MessageCursor
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		lastCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}
	}
	var oldestCursor *types.MessageCursor
	if len(rawMessages) > 0 {
		first := rawMessages[0]
		oldestCursor = &types.MessageCursor{GUID: first.ID, TS: first.TS}
	}

	// Initialize mention cursor from persisted watermark to avoid re-notifying
	var lastMentionCursor *types.MessageCursor
	if opts.Username != "" {
		if mentionWatermark, _ := db.GetReadTo(opts.DB, opts.Username, "mentions"); mentionWatermark != nil {
			lastMentionCursor = &types.MessageCursor{GUID: mentionWatermark.MessageGUID, TS: mentionWatermark.MessageTS}
		}
	}

	count, err := countMessages(opts.DB, opts.IncludeArchived)
	if err != nil {
		return nil, err
	}

	model := &Model{
		db:              opts.DB,
		projectName:     opts.ProjectName,
		projectRoot:     opts.ProjectRoot,
		projectDBPath:   opts.ProjectDBPath,
		username:        opts.Username,
		showUpdates:     opts.ShowUpdates,
		includeArchived: opts.IncludeArchived,
		viewport:        vp,
		input:           input,
		messages:        messages,
		lastCursor:        lastCursor,
		lastMentionCursor: lastMentionCursor,
		oldestCursor:      oldestCursor,
		status:            "",
		messageCount:    count,
		lastLimit:       opts.Last,
		hasMore:         len(rawMessages) >= opts.Last,
		colorMap:        colorMap,
		threads:            threads,
		threadIndex:        threadIndex,
		threadPanelOpen: true,
		sidebarOpen:     false,
		visitedThreads:     make(map[string]types.Thread),
		unreadCounts:       make(map[string]int),
		collapsedThreads:   make(map[string]bool),
		favedThreads:       make(map[string]bool),
		subscribedThreads:  make(map[string]bool),
		mutedThreads:       make(map[string]bool),
		threadNicknames:    make(map[string]string),
		avatarMap:          make(map[string]string),
		agentUnreadCounts:     make(map[string]int),
		agentDisplayPresence:  make(map[string]types.PresenceState),
		agentActualPresence:   make(map[string]types.PresenceState),
		agentPresenceChanged:  make(map[string]time.Time),
		expandedJobClusters:   make(map[string]bool),
		managedAgents:         managedAgents,
		agentTokenUsage:       agentTokenUsage,
		statusInvoker:      NewStatusInvoker(filepath.Join(opts.ProjectRoot, ".fray")),
		zoneManager:        zone.New(),
		channels:        channels,
		channelIndex:    channelIndex,
		initialScroll:   true,
		debugSync:       opts.DebugSync,
	}
	if opts.DebugSync {
		model.stopSyncChecker = make(chan struct{})
		go model.runSyncChecker()
	}
	model.refreshQuestionCounts()
	model.refreshUnreadCounts()
	model.refreshFavedThreads()
	model.refreshSubscribedThreads()
	model.refreshMutedThreads()
	model.refreshThreadNicknames()
	model.refreshAvatars()
	model.calculateThreadPanelWidth() // Calculate initial width since panel starts open
	return model, nil
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.pollCmd(), m.activityPollCmd())
}

func (m *Model) Close() {
	if m.stopSyncChecker != nil {
		close(m.stopSyncChecker)
	}
	if m.db != nil {
		_ = m.db.Close()
	}
}

// runSyncChecker polls DB every 5s and logs when in-memory state diverges from reality.
// Used to debug #fray-rvuu (chat session stops showing new messages/threads).
func (m *Model) runSyncChecker() {
	logPath := filepath.Join(m.projectRoot, ".fray", "sync-debug.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[debug-sync] failed to open log: %v\n", err)
		return
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "[%s] sync checker started\n", time.Now().Format(time.RFC3339))

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopSyncChecker:
			fmt.Fprintf(logFile, "[%s] sync checker stopped\n", time.Now().Format(time.RFC3339))
			return
		case <-ticker.C:
			m.checkSync(logFile)
		}
	}
}

// checkSync compares in-memory state to DB and logs discrepancies.
func (m *Model) checkSync(logFile *os.File) {
	now := time.Now().Format(time.RFC3339)

	// Get recent messages from DB (last 100)
	dbMsgs, err := db.GetMessages(m.db, &types.MessageQueryOptions{
		Limit:           100,
		IncludeArchived: m.includeArchived,
	})
	if err != nil {
		fmt.Fprintf(logFile, "[%s] ERROR: failed to query messages: %v\n", now, err)
		return
	}

	// Build map of in-memory message IDs
	memMsgIDs := make(map[string]bool)
	for _, msg := range m.messages {
		memMsgIDs[msg.ID] = true
	}

	// Find messages in DB but not in memory
	var missingMsgs []string
	for _, dbMsg := range dbMsgs {
		if !memMsgIDs[dbMsg.ID] {
			missingMsgs = append(missingMsgs, dbMsg.ID)
		}
	}

	if len(missingMsgs) > 0 {
		fmt.Fprintf(logFile, "[%s] DESYNC: %d messages in DB not in chat (mem=%d, db=%d): %v\n",
			now, len(missingMsgs), len(m.messages), len(dbMsgs), missingMsgs)
	}

	// Check threads
	dbThreads, err := db.GetThreads(m.db, nil)
	if err != nil {
		fmt.Fprintf(logFile, "[%s] ERROR: failed to query threads: %v\n", now, err)
		return
	}

	memThreadIDs := make(map[string]bool)
	for _, t := range m.threads {
		memThreadIDs[t.GUID] = true
	}

	var missingThreads []string
	for _, dbThread := range dbThreads {
		if !memThreadIDs[dbThread.GUID] {
			missingThreads = append(missingThreads, dbThread.GUID)
		}
	}

	if len(missingThreads) > 0 {
		fmt.Fprintf(logFile, "[%s] DESYNC: %d threads in DB not in chat (mem=%d, db=%d): %v\n",
			now, len(missingThreads), len(m.threads), len(dbThreads), missingThreads)
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case tea.KeyMsg:
		if handled, cmd := m.handleSuggestionKeys(msg); handled {
			return m, cmd
		}
		if handled, cmd := m.handleThreadPanelKeys(msg); handled {
			return m, cmd
		}
		if handled, cmd := m.handleSidebarKeys(msg); handled {
			return m, cmd
		}
		if msg.Type == tea.KeyRunes && !msg.Paste && msg.String() == "?" && m.input.Value() == "" {
			m.showHelp()
			return m, nil
		}
		if msg.Type == tea.KeyUp && m.input.Value() == "" {
			if m.prefillEditCommand() {
				return m, nil
			}
		}
		if msg.Type == tea.KeyCtrlJ {
			m.insertInputText("\n")
			return m, nil
		}
		if msg.Type == tea.KeyRunes && !msg.Paste && strings.ContainsRune(string(msg.Runes), '\n') {
			m.insertInputText(normalizeNewlines(string(msg.Runes)))
			return m, nil
		}
		if msg.Type == tea.KeyRunes && msg.Paste {
			m.insertInputText(normalizeNewlines(string(msg.Runes)))
			return m, nil
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.input.Value() != "" || m.replyToID != "" {
				m.input.Reset()
				m.clearSuggestions()
				// Clear reply reference without status message (silent clear)
				m.replyToID = ""
				m.replyToPreview = ""
				m.lastInputValue = m.input.Value()
				m.lastInputPos = m.inputCursorPos()
				m.updateInputStyle()
				m.resize()
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEsc:
			if m.editingMessageID != "" {
				m.exitEditMode()
				m.resize()
				return m, nil
			}
			// Clear peek mode if active
			if m.isPeeking() {
				m.clearPeek()
				m.refreshViewport(false)
				return m, nil
			}
			// Clear reply reference if set
			if m.replyToID != "" {
				m.clearReplyTo()
				return m, nil
			}
		case tea.KeyBackspace, tea.KeyDelete:
			// Backspace at position 0 with reply set: clear reply (regardless of text content)
			if m.replyToID != "" && m.inputCursorPos() == 0 {
				m.clearReplyTo()
				return m, nil
			}
		case tea.KeyEnter:
			// Handle edit mode submission
			if m.editingMessageID != "" {
				value := strings.TrimSpace(m.input.Value())
				if value == "" {
					m.status = "Cannot save empty message"
					return m, nil
				}
				msgID := m.editingMessageID
				m.exitEditMode()
				m.resize()
				return m, m.submitEdit(msgID, value)
			}
			// DEBUG: log replyToID state at Enter press time
			debugLog(fmt.Sprintf("KeyEnter: replyToID=%q replyToPreview=%q", m.replyToID, m.replyToPreview))
			value := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			m.clearSuggestions()
			m.lastInputValue = m.input.Value()
			m.lastInputPos = m.inputCursorPos()
			m.updateInputStyle()
			m.resize()
			if value == "" {
				return m, nil
			}
			if handled, cmd := m.handleSlashCommand(value); handled {
				// Slash commands don't clear the new message notification
				return m, cmd
			}
			// Regular messages clear the new message notification and scroll to bottom
			m.clearNewMessageNotification()
			m.refreshViewport(true)
			return m, m.handleSubmit(value)
		case tea.KeyShiftTab:
			// Shift-Tab: open channels panel
			m.openChannelPanel()
			return m, nil
		case tea.KeyTab:
			// Tab: open threads panel
			if len(m.suggestions) > 0 {
				return m, nil
			}
			m.openThreadPanel()
			return m, nil
		case tea.KeyCtrlB:
			// Ctrl-B: toggle sidebar persistence
			m.toggleSidebarPersistence()
			return m, nil
		case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			if (msg.Type == tea.KeyPgUp || msg.Type == tea.KeyHome) && m.nearTop() {
				m.loadOlderMessages()
			}
			// Clear new message notification if scrolled to bottom
			if m.atBottom() {
				m.clearNewMessageNotification()
			}
			return m, cmd
		}
		var cmd tea.Cmd
		// Allow input when: no panel focus OR peeking (and not in filter mode)
		// This lets you type while peeking without losing the peek context
		canType := !m.sidebarFocus && !m.threadPanelFocus
		canType = canType || (m.isPeeking() && !m.threadFilterActive)
		if canType {
			cmd = m.safeInputUpdate(msg)
			m.refreshSuggestions()
			m.resize()
		}
		return m, cmd
	case tea.MouseMsg:
		if msg.Shift {
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if handled, cmd := m.handleMouseClick(msg); handled {
				return m, cmd
			}
		}
		// Handle mouse wheel scrolling based on cursor position
		threadWidth := m.threadPanelWidth()
		sidebarWidth := m.sidebarWidth()
		isWheelUp := msg.Button == tea.MouseButtonWheelUp
		isWheelDown := msg.Button == tea.MouseButtonWheelDown
		if isWheelUp || isWheelDown {
			// Check if over thread panel
			if m.threadPanelOpen && msg.X < threadWidth {
				if isWheelUp {
					m.threadScrollOffset--
					if m.threadScrollOffset < 0 {
						m.threadScrollOffset = 0
					}
				} else {
					m.threadScrollOffset++
				}
				return m, nil
			}
			// Check if over sidebar
			if m.sidebarOpen && msg.X >= threadWidth && msg.X < threadWidth+sidebarWidth {
				if isWheelUp {
					m.sidebarScrollOffset--
					if m.sidebarScrollOffset < 0 {
						m.sidebarScrollOffset = 0
					}
				} else {
					m.sidebarScrollOffset++
				}
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if msg.Button == tea.MouseButtonWheelUp && m.nearTop() {
			m.loadOlderMessages()
		}
		// Clear new message notification if scrolled to bottom
		if m.atBottom() {
			m.clearNewMessageNotification()
		}
		return m, cmd
	case pollMsg:
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
							m.addNewMessageAuthor(incomingMsg.FromAgent)
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
	case activityPollMsg:
		// Fast poll for activity panel updates (250ms)
		m.animationFrame++ // Increment animation frame for spawn cycle animation
		if msg.managedAgents != nil {
			// Track presence changes for debouncing (suppress flicker)
			now := time.Now()
			const presenceDebounceMs = 1000 // 1 second debounce
			for i := range msg.managedAgents {
				agent := &msg.managedAgents[i]
				actualPresence, hasActual := m.agentActualPresence[agent.AgentID]
				if !hasActual {
					// First time seeing this agent - initialize both to current
					m.agentActualPresence[agent.AgentID] = agent.Presence
					m.agentDisplayPresence[agent.AgentID] = agent.Presence
					m.agentPresenceChanged[agent.AgentID] = now
				} else if agent.Presence != actualPresence {
					// Actual presence changed - record change time
					m.agentActualPresence[agent.AgentID] = agent.Presence
					m.agentPresenceChanged[agent.AgentID] = now
				}
				// Update display presence if debounce period has passed
				changedAt := m.agentPresenceChanged[agent.AgentID]
				if now.Sub(changedAt).Milliseconds() >= presenceDebounceMs {
					m.agentDisplayPresence[agent.AgentID] = m.agentActualPresence[agent.AgentID]
				}
			}
			m.managedAgents = msg.managedAgents
		}
		if msg.agentTokenUsage != nil {
			m.agentTokenUsage = msg.agentTokenUsage
		}
		// Detect daemon restart and trigger database refresh
		if msg.daemonStartedAt != 0 && msg.daemonStartedAt != m.daemonStartedAt {
			if m.daemonStartedAt != 0 {
				// Daemon restarted - reload messages to get fresh data
				m.status = "Daemon restarted, refreshing..."
				if err := m.reloadMessages(); err == nil {
					m.status = ""
				}
			}
			m.daemonStartedAt = msg.daemonStartedAt
		}
		return m, m.activityPollCmd()
	case errMsg:
		m.status = msg.err.Error()
		return m, m.pollCmd()
	case editResultMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Edit failed: %v", msg.err)
			return m, nil
		}
		if msg.msg != nil {
			reason := "edit"
			if err := m.appendMessageEditUpdate(*msg.msg, reason); err != nil {
				m.status = fmt.Sprintf("Edit failed: %v", err)
				return m, nil
			}
			annotated, err := db.ApplyMessageEditCounts(m.projectDBPath, []types.Message{*msg.msg})
			if err == nil && len(annotated) > 0 {
				*msg.msg = annotated[0]
			}
			m.applyMessageUpdate(*msg.msg)
			m.refreshViewport(false)
			m.status = fmt.Sprintf("Edited #%s", msg.msg.ID)
		}
		return m, nil
	case clickDebounceMsg:
		// Execute pending single-click if it matches and hasn't been superseded
		if m.pendingClick != nil &&
			m.pendingClick.messageID == msg.messageID &&
			m.pendingClick.timestamp == msg.timestamp {
			m.executePendingClick()
		}
		return m, nil
	}

	cmd := m.safeInputUpdate(msg)
	m.refreshSuggestions()
	return m, cmd
}

func (m *Model) View() string {
	statusLine := lipgloss.NewStyle().Foreground(statusColor).Render(m.statusLine())

	var lines []string
	// Add pinned permission requests at top
	if pinnedPerms := m.renderPinnedPermissions(); pinnedPerms != "" {
		lines = append(lines, pinnedPerms)
	}
	// Add peek statusline at top if peeking
	if peekTop := m.renderPeekStatusline(); peekTop != "" {
		lines = append(lines, peekTop)
	}
	lines = append(lines, m.viewport.View())
	if suggestions := m.renderSuggestions(); suggestions != "" {
		lines = append(lines, suggestions)
	}
	// Add peek statusline above input if peeking, otherwise add margin
	if peekBottom := m.renderPeekStatusline(); peekBottom != "" {
		lines = append(lines, peekBottom)
	} else {
		lines = append(lines, "") // margin line when not peeking
	}
	lines = append(lines, m.renderInput(), statusLine)

	main := lipgloss.JoinVertical(lipgloss.Left, lines...)
	panels := make([]string, 0, 2)
	if m.threadPanelOpen {
		if panel := m.renderThreadPanel(); panel != "" {
			panels = append(panels, panel)
		}
	}
	if m.sidebarOpen {
		if panel := m.renderSidebar(); panel != "" {
			panels = append(panels, panel)
		}
	}
	var output string
	if len(panels) == 0 {
		output = main
	} else {
		left := lipgloss.JoinHorizontal(lipgloss.Top, panels...)
		output = lipgloss.JoinHorizontal(lipgloss.Top, left, main)
	}
	return m.zoneManager.Scan(output)
}

func (m *Model) renderInput() string {
	var parts []string

	// Add new messages notification bar if user has scrolled up
	if newMsgBar := m.renderNewMessagesBar(); newMsgBar != "" {
		parts = append(parts, newMsgBar)
	}

	// Add reply preview if replying
	if replyPreview := m.renderReplyPreview(); replyPreview != "" {
		parts = append(parts, replyPreview)
	}

	content := m.input.View()
	style := lipgloss.NewStyle().Background(inputBg).Padding(0, inputPadding, 0, 0)
	if width := m.mainWidth(); width > 0 {
		style = style.Width(width)
	}
	blank := style.Render("")
	parts = append(parts, blank, style.Render(content), blank)
	return strings.Join(parts, "\n")
}

// renderReplyPreview renders the reply preview line above the input when replying
func (m *Model) renderReplyPreview() string {
	if m.replyToID == "" {
		return ""
	}

	// Style similar to reply context in messages
	previewStyle := lipgloss.NewStyle().Foreground(metaColor).Italic(true)
	cancelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	preview := previewStyle.Render(fmt.Sprintf("↪ Replying to: %s", m.replyToPreview))
	cancel := m.zoneManager.Mark("reply-cancel", cancelStyle.Render(" [x]"))

	width := m.mainWidth()
	if width > 0 {
		// Calculate padding to right-align the cancel button
		previewWidth := lipgloss.Width(preview)
		cancelWidth := lipgloss.Width(cancel)
		padding := width - previewWidth - cancelWidth
		if padding > 0 {
			return preview + strings.Repeat(" ", padding) + cancel
		}
	}
	return preview + " " + cancel
}

// renderNewMessagesBar renders a blue notification bar when new messages arrive while scrolled up
func (m *Model) renderNewMessagesBar() string {
	if len(m.newMessageAuthors) == 0 {
		return ""
	}

	// Build message text
	var text string
	if len(m.newMessageAuthors) == 1 {
		text = fmt.Sprintf("new message from @%s", m.newMessageAuthors[0])
	} else {
		mentions := make([]string, len(m.newMessageAuthors))
		for i, author := range m.newMessageAuthors {
			mentions[i] = "@" + author
		}
		text = fmt.Sprintf("new messages from %s", strings.Join(mentions, " "))
	}

	barStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).  // white text
		Background(lipgloss.Color("24")).   // blue background (same as peek mode)
		Padding(0, 1)

	width := m.mainWidth()
	if width > 0 {
		barStyle = barStyle.Width(width)
	}

	return barStyle.Render(text)
}

// renderPeekStatusline renders a blue statusline for peek mode.
// Shows: "peeking <thread-name> · <action-hint>"
func (m *Model) renderPeekStatusline() string {
	if !m.isPeeking() {
		return ""
	}

	// Build thread name
	var name string
	if m.peekThread != nil {
		name = m.peekThread.Name
	} else if m.peekPseudo != "" {
		name = string(m.peekPseudo)
	} else {
		name = m.projectName // peeking main room
	}

	// Build action hint based on peek source
	hint := ""
	switch m.peekSource {
	case peekSourceKeyboard:
		hint = "enter to open"
	case peekSourceClick:
		hint = "click to open"
	}

	// Build content
	content := "peeking " + name
	if hint != "" {
		content += " · " + hint
	}

	// Style with blue background
	style := lipgloss.NewStyle().
		Background(peekBg).
		Foreground(lipgloss.Color("231")).
		Padding(0, 1)

	if width := m.mainWidth(); width > 0 {
		style = style.Width(width)
	}

	return style.Render(content)
}

func (m *Model) statusLine() string {
	right := ""
	if m.input.Value() == "" {
		right = "? for help"
	}
	breadcrumb := m.breadcrumb()
	left := breadcrumb
	if m.status != "" {
		left = fmt.Sprintf("%s · %s", m.status, breadcrumb)
	}
	return alignStatusLine(left, right, m.mainWidth())
}

func alignStatusLine(left, right string, width int) string {
	if width <= 0 || right == "" {
		return left
	}
	leftWidth := ansi.StringWidth(left)
	rightWidth := ansi.StringWidth(right)
	if leftWidth+rightWidth+1 > width {
		return left
	}
	spaces := width - leftWidth - rightWidth
	return left + strings.Repeat(" ", spaces) + right
}

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

func (m *Model) removeMessageByID(id string) bool {
	for i, msg := range m.messages {
		if msg.ID != id {
			continue
		}
		m.messages = append(m.messages[:i], m.messages[i+1:]...)
		return true
	}
	return false
}

func (m *Model) handleMouseClick(msg tea.MouseMsg) (bool, tea.Cmd) {
	debugLog(fmt.Sprintf("handleMouseClick: action=%v button=%v x=%d y=%d replyToID=%q", msg.Action, msg.Button, msg.X, msg.Y, m.replyToID))
	// Check for reply cancel click first (anywhere on screen)
	if m.replyToID != "" && m.zoneManager.Get("reply-cancel").InBounds(msg) {
		debugLog("handleMouseClick: clearing reply via cancel zone")
		m.clearReplyTo()
		return true, nil
	}

	// Check for pinned permission request button clicks (at top of screen)
	for _, message := range m.messages {
		if event := parseInteractiveEvent(message); event != nil {
			if event.Kind == "permission" && (event.Status == "" || event.Status == "pending") {
				for _, action := range event.Actions {
					zoneID := fmt.Sprintf("pinned-action-%s-%s", message.ID, action.ID)
					if m.zoneManager.Get(zoneID).InBounds(msg) {
						debugLog(fmt.Sprintf("handleMouseClick: pinned button clicked: %s", zoneID))
						if action.Command != "" {
							go m.executeActionCommand(action.Command)
							m.status = fmt.Sprintf("Executing: %s", action.Label)
						}
						return true, nil
					}
				}
			}
		}
	}

	threadWidth := 0
	if m.threadPanelOpen {
		threadWidth = m.threadPanelWidth()
		if msg.X < threadWidth {
			// Check for job cluster zone clicks (activity panel)
			for _, agent := range m.managedAgents {
				if agent.JobID != nil && *agent.JobID != "" {
					zoneID := "job-cluster-" + *agent.JobID
					if m.zoneManager.Get(zoneID).InBounds(msg) {
						// Toggle cluster expansion
						m.expandedJobClusters[*agent.JobID] = !m.expandedJobClusters[*agent.JobID]
						return true, nil
					}
				}
			}

			// Check for agent zone clicks (activity panel at bottom)
			for _, agent := range m.managedAgents {
				zoneID := "agent-" + agent.AgentID
				if m.zoneManager.Get(zoneID).InBounds(msg) {
					// Navigate to agent's last posted thread
					m.navigateToAgentThread(agent.AgentID)
					return true, nil
				}
			}

			if msg.Y < lipgloss.Height(m.renderThreadPanel()) {
				if index := m.threadIndexAtLine(msg.Y); index >= 0 {
					if index == m.threadIndex {
						// Clicking same index
						if m.isPeeking() {
							// If peeking, click confirms selection (gives panel focus)
							m.threadPanelFocus = true
							m.sidebarFocus = false
							m.updateInputFocus()
							m.selectThreadEntry()
						} else {
							// If not peeking (already in this thread), drill in
							m.threadPanelFocus = true
							m.sidebarFocus = false
							m.updateInputFocus()
							m.drillInAction()
						}
					} else {
						// Clicking different index - peek it without grabbing focus
						// This allows the user to keep typing while previewing content
						m.threadIndex = index
						m.peekThreadEntry(peekSourceClick)
					}
					return true, nil
				}
				// Click on header when drilled in -> drill out
				// Header is at Y = pinnedPermissionsHeight (first line after top padding)
				if msg.Y == m.pinnedPermissionsHeight() && m.drillDepth() > 0 {
					m.drillOutAction()
					return true, nil
				}
				// Clicked elsewhere in thread panel - give it focus for navigation
				m.threadPanelFocus = true
				m.sidebarFocus = false
				m.updateInputFocus()
				return true, nil
			}
			// Clicked below content area in thread panel
			m.threadPanelFocus = true
			m.sidebarFocus = false
			m.updateInputFocus()
			return true, nil
		}
	}

	if m.sidebarOpen {
		sidebarStart := threadWidth
		if msg.X >= sidebarStart && msg.X < sidebarStart+m.sidebarWidth() {
			// Clicked in sidebar - give it focus
			m.sidebarFocus = true
			m.threadPanelFocus = false
			m.updateInputFocus()
			if msg.Y < lipgloss.Height(m.renderSidebar()) {
				if index := m.sidebarIndexAtLine(msg.Y); index >= 0 {
					m.channelIndex = index
					return true, m.selectChannelCmd()
				}
				return true, nil
			}
			return true, nil
		}
	}

	// Clicked in main area - focus textarea
	m.threadPanelFocus = false
	m.sidebarFocus = false
	m.updateInputFocus()

	// Account for peek statusline and pinned permissions at top when calculating viewport Y
	topOffset := 0
	if m.isPeeking() {
		topOffset = 1 // peek statusline takes 1 row
	}
	topOffset += m.pinnedPermissionsHeight()
	viewportY := msg.Y - topOffset

	if viewportY < 0 || viewportY >= m.viewport.Height {
		// Clicking outside viewport (peek statusline or textarea area) - just focus input, keep peek state
		return false, nil
	}

	line := m.viewport.YOffset + viewportY

	// Check for subthread zone clicks before checking messages
	if m.currentThread != nil {
		children, _ := db.GetChildThreadsWithStats(m.db, m.currentThread.GUID)
		for _, child := range children {
			zoneID := "subthread-" + child.GUID
			if m.zoneManager.Get(zoneID).InBounds(msg) {
				m.navigateToThread(child.GUID)
				return true, nil
			}
		}
	}

	message, ok := m.messageAtLine(line)
	if !ok || message == nil {
		return ok, nil
	}

	// Check for interactive action button clicks first
	if event := parseInteractiveEvent(*message); event != nil {
		debugLog(fmt.Sprintf("handleMouseClick: message %s is interactive event with %d actions", message.ID, len(event.Actions)))
		for _, action := range event.Actions {
			zoneID := fmt.Sprintf("action-%s-%s", message.ID, action.ID)
			zone := m.zoneManager.Get(zoneID)
			debugLog(fmt.Sprintf("handleMouseClick: checking zone %s, InBounds=%v", zoneID, zone.InBounds(msg)))
			if zone.InBounds(msg) {
				if action.Command != "" {
					debugLog(fmt.Sprintf("handleMouseClick: executing command: %s", action.Command))
					go m.executeActionCommand(action.Command)
					m.status = fmt.Sprintf("Executing: %s", action.Label)
				}
				return true, nil
			}
		}
	}

	now := time.Now()
	isDoubleClick := m.lastClickID == message.ID && now.Sub(m.lastClickAt) <= doubleClickInterval

	// Check if clicked on the GUID zone (footer message ID)
	guidZone := fmt.Sprintf("guid-%s", message.ID)
	clickedOnGUID := m.zoneManager.Get(guidZone).InBounds(msg)

	if isDoubleClick {
		// Clear double-click tracking and cancel any pending single-click
		m.lastClickID = ""
		m.lastClickAt = time.Time{}
		m.pendingClick = nil

		// Commit peek on double-click action
		if m.isPeeking() {
			m.commitPeek()
		}
		if clickedOnGUID {
			// Double-click on footer ID: set reply reference (no clipboard copy)
			m.setReplyTo(*message)
		} else {
			// Double-click elsewhere: copy from zone (text sections)
			m.copyFromZone(msg, *message)
		}
		return true, nil
	}

	// Single-click - use debounce for GUID clicks
	m.lastClickID = message.ID
	m.lastClickAt = now

	if clickedOnGUID {
		// Queue single-click on GUID for debounce (will copy ID after delay)
		m.pendingClick = &pendingClick{
			messageID: message.ID,
			zone:      "guid",
			text:      message.ID,
			timestamp: now,
		}
		// Start debounce timer
		return true, m.clickDebounceCmd(message.ID, now)
	}

	// Non-GUID single-click: handle immediately (no debounce needed)
	if m.isPeeking() {
		// Single-click on message content while peeking: commit peek (switch to this view)
		m.commitPeek()
	}
	// Single-click elsewhere when not peeking: do nothing (wait for possible double-click)
	return true, nil
}

// clickDebounceCmd returns a command that fires after singleClickDebounce
func (m *Model) clickDebounceCmd(messageID string, timestamp time.Time) tea.Cmd {
	return tea.Tick(singleClickDebounce, func(time.Time) tea.Msg {
		return clickDebounceMsg{messageID: messageID, timestamp: timestamp}
	})
}

// executePendingClick executes the pending single-click action
func (m *Model) executePendingClick() {
	if m.pendingClick == nil {
		return
	}
	pc := m.pendingClick
	m.pendingClick = nil

	// Commit peek since user is interacting with content
	if m.isPeeking() {
		m.commitPeek()
	}

	switch pc.zone {
	case "guid":
		// Copy the message ID to clipboard
		if err := copyToClipboard(pc.text); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Copied message ID to clipboard."
		}
	case "inlineid":
		// Copy the inline ID (without # prefix)
		if err := copyToClipboard(pc.text); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Copied ID to clipboard."
		}
	}
}

// setReplyTo sets the reply reference and shows preview (called on double-click footer ID)
func (m *Model) setReplyTo(msg types.Message) {
	debugLog(fmt.Sprintf("setReplyTo: msgID=%s from=%s", msg.ID, msg.FromAgent))
	m.replyToID = msg.ID
	m.replyToPreview = truncatePreview(msg.Body, 40)
	m.status = fmt.Sprintf("Replying to @%s", msg.FromAgent)
	m.resize() // Recalculate layout to account for reply preview line
}

// clearReplyTo clears the reply reference (called on Esc or backspace at pos 0)
func (m *Model) clearReplyTo() {
	// Debug: log caller information
	pc := make([]uintptr, 10)
	n := runtime.Callers(2, pc)
	frames := runtime.CallersFrames(pc[:n])
	var callers []string
	for {
		frame, more := frames.Next()
		callers = append(callers, fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line))
		if !more || len(callers) >= 3 {
			break
		}
	}
	debugLog(fmt.Sprintf("clearReplyTo called from: %v, replyToID was: %q", callers, m.replyToID))

	m.replyToID = ""
	m.replyToPreview = ""
	m.status = "Reply cancelled"
	m.resize() // Recalculate layout now that reply preview is gone
}

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

func (m *Model) copyFromZone(mouseMsg tea.MouseMsg, msg types.Message) {
	// Check which zone was clicked (GUID zone is handled separately for reply insertion)
	bylineZone := fmt.Sprintf("byline-%s", msg.ID)
	footerZone := fmt.Sprintf("footer-%s", msg.ID)

	var textToCopy string
	var description string

	// Check each zone type in priority order
	// Note: All text copies EXCLUDE speaker name per spec
	if m.zoneManager.Get(bylineZone).InBounds(mouseMsg) || m.zoneManager.Get(footerZone).InBounds(mouseMsg) {
		// Double-clicked on byline or footer - copy whole message body (no speaker)
		textToCopy = msg.Body
		description = "message"
	} else {
		// Check inline ID zones first (more specific than line zones)
		foundInlineID := false
		lines := strings.Split(msg.Body, "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			matches := inlineIDPattern.FindAllString(line, -1)
			for idx, idMatch := range matches {
				inlineIDZone := fmt.Sprintf("inlineid-%s-%d-%d", msg.ID, i, idx)
				if m.zoneManager.Get(inlineIDZone).InBounds(mouseMsg) {
					// Copy the ID without the # prefix
					textToCopy = idMatch[1:]
					description = "ID"
					foundInlineID = true
					break
				}
			}
			if foundInlineID {
				break
			}
		}

		if !foundInlineID {
			// Check line zones
			foundLine := false
			for i, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue // blank lines don't have zones
				}
				lineZone := fmt.Sprintf("line-%s-%d", msg.ID, i)
				if m.zoneManager.Get(lineZone).InBounds(mouseMsg) {
					textToCopy = line
					description = "line"
					foundLine = true
					break
				}
			}
			if !foundLine {
				// Fallback: copy whole message body (no speaker)
				textToCopy = msg.Body
				description = "message"
			}
		}
	}

	if err := copyToClipboard(textToCopy); err != nil {
		m.status = err.Error()
		return
	}
	m.status = fmt.Sprintf("Copied %s to clipboard.", description)
}


func (m *Model) copyMessage(msg types.Message) {
	// Copy message body without speaker name per spec
	if err := copyToClipboard(msg.Body); err != nil {
		m.status = err.Error()
		return
	}
	m.status = "Copied message to clipboard."
}

func (m *Model) channelLabel() string {
	for _, entry := range m.channels {
		if samePath(entry.Path, m.projectRoot) {
			if entry.Name != "" {
				return entry.Name
			}
			if entry.ID != "" {
				return entry.ID
			}
		}
	}
	if m.projectName != "" {
		return m.projectName
	}
	return "channel"
}

func (m *Model) currentThreadLabel() string {
	if m.currentPseudo != "" {
		return string(m.currentPseudo)
	}
	if m.currentThread == nil {
		return "#main"
	}
	path, err := threadPath(m.db, m.currentThread)
	if err != nil || path == "" {
		return m.currentThread.GUID
	}
	return path
}

func (m *Model) breadcrumb() string {
	channel := m.channelLabel()
	if m.currentPseudo != "" {
		return channel + " ❯ " + string(m.currentPseudo)
	}
	if m.currentThread == nil {
		return channel + " ❯ main"
	}
	path, err := threadPath(m.db, m.currentThread)
	if err != nil || path == "" {
		return channel + " ❯ " + m.currentThread.GUID
	}
	// Convert slash-separated path to breadcrumb with ❯
	parts := strings.Split(path, "/")
	return channel + " ❯ " + strings.Join(parts, " ❯ ")
}

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

func (m *Model) refreshFavedThreads() {
	if m.db == nil || m.username == "" {
		return
	}
	guids, err := db.GetFavedThreads(m.db, m.username)
	if err != nil {
		return
	}
	m.favedThreads = make(map[string]bool)
	for _, guid := range guids {
		m.favedThreads[guid] = true
	}
}

func (m *Model) refreshSubscribedThreads() {
	if m.db == nil || m.username == "" {
		return
	}
	threads, err := db.GetThreads(m.db, &types.ThreadQueryOptions{
		SubscribedAgent: &m.username,
	})
	if err != nil {
		return
	}
	m.subscribedThreads = make(map[string]bool)
	for _, t := range threads {
		m.subscribedThreads[t.GUID] = true
	}
}

func (m *Model) refreshMutedThreads() {
	if m.db == nil || m.username == "" {
		return
	}
	guids, err := db.GetMutedThreadGUIDs(m.db, m.username)
	if err != nil {
		return
	}
	m.mutedThreads = guids
}

func (m *Model) refreshThreadNicknames() {
	if m.db == nil || m.username == "" {
		return
	}
	nicknames, err := db.GetThreadNicknames(m.db, m.username)
	if err != nil {
		return
	}
	m.threadNicknames = nicknames
}

func (m *Model) refreshAvatars() {
	if m.db == nil {
		return
	}
	agents, err := db.GetAgents(m.db)
	if err != nil {
		return
	}
	m.avatarMap = make(map[string]string)
	for _, agent := range agents {
		if agent.Avatar != nil && *agent.Avatar != "" {
			m.avatarMap[agent.AgentID] = *agent.Avatar
		}
	}
}

func countMessages(dbConn *sql.DB, includeArchived bool) (int, error) {
	query := "SELECT COUNT(*) FROM fray_messages"
	if !includeArchived {
		query += " WHERE archived_at IS NULL"
	}
	row := dbConn.QueryRow(query)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}


func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

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

// getPendingPermissionGUIDs returns a map of permission GUIDs that are truly pending
// (not yet approved/denied in permissions.jsonl).
func (m *Model) getPendingPermissionGUIDs() map[string]bool {
	pending := make(map[string]bool)
	perms, err := db.ReadPermissions(m.projectRoot)
	if err != nil {
		return pending
	}
	for _, perm := range perms {
		if perm.Status == types.PermissionStatusPending || perm.Status == "" {
			pending[perm.GUID] = true
		}
	}
	return pending
}

// pinnedPermissionsHeight returns the number of lines used by pinned permission requests.
// Currently disabled - returns 0 to avoid layout complexity.
// Permission requests are shown inline in the viewport instead.
func (m *Model) pinnedPermissionsHeight() int {
	// DISABLED: Pinned permissions cause layout complexity.
	// Permission requests are still shown inline in the viewport.
	return 0
}
