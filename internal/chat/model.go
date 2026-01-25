package chat

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
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
	db                     *sql.DB
	projectName            string
	projectRoot            string
	projectDBPath          string
	username               string
	showUpdates            bool
	includeArchived        bool
	viewport               viewport.Model
	input                  textarea.Model
	messages               []types.Message
	lastCursor             *types.MessageCursor
	lastMentionCursor      *types.MessageCursor
	oldestCursor           *types.MessageCursor
	status                 string
	width                  int
	height                 int
	messageCount           int
	lastLimit              int
	hasMore                bool
	colorMap               map[string]lipgloss.Color
	suggestions            []suggestionItem
	suggestionIndex        int
	suggestionStart        int
	suggestionKind         suggestionKind
	reactionMode           bool
	replyMode              bool
	wasEditMode            bool   // true if last style update was in edit mode
	editingMessageID       string // non-empty when in edit mode
	lastInputValue         string
	lastInputPos           int
	threads                []types.Thread
	threadIndex            int
	threadPanelOpen        bool
	threadPanelFocus       bool
	cachedThreadPanelWidth int // calculated width (snapshot when panel opens)
	threadFilter           string
	threadMatches          []int
	threadFilterActive     bool
	threadSearchResults    []types.Thread
	recentThreads          []types.Thread
	visitedThreads         map[string]types.Thread
	currentThread          *types.Thread
	currentPseudo          pseudoThreadKind
	threadMessages         []types.Message
	questionCounts         map[pseudoThreadKind]int
	pseudoQuestions        []types.Question
	unreadCounts           map[string]int    // unread message counts per thread GUID
	roomUnreadCount        int               // unread count for main room
	collapsedThreads       map[string]bool   // collapsed state per thread GUID
	favedThreads           map[string]bool   // faved threads for current user
	subscribedThreads      map[string]bool   // subscribed threads for current user
	mutedThreads           map[string]bool   // muted threads for current user
	viewingMutedCollection bool              // true when drilled into muted collection view
	threadNicknames        map[string]string // thread nicknames for current user
	avatarMap              map[string]string // agent_id -> avatar character
	drillPath              []string          // current drill path (thread GUIDs from root to current)
	threadScrollOffset     int               // scroll offset for virtual scrolling in thread panel
	zoneManager            *zone.Manager     // bubblezone manager for click tracking
	channels               []channelEntry
	channelIndex           int
	sidebarOpen            bool
	sidebarFocus           bool
	sidebarFilter          string
	sidebarMatches         []int
	sidebarFilterActive    bool
	sidebarScrollOffset    int    // scroll offset for virtual scrolling in channel sidebar
	sidebarPersistent      bool   // if true, Tab just changes focus (doesn't close)
	pendingNicknameGUID    string // thread GUID for pending /n command (set by Ctrl-N)
	helpMessageID          string
	initialScroll          bool
	pendingScrollBottom    bool // scroll to bottom on next viewport refresh
	lastClickID            string
	lastClickAt            time.Time
	// Activity panel state
	managedAgents        []types.Agent          // daemon-managed agents
	agentUnreadCounts    map[string]int         // unread count per agent
	agentTokenUsage      map[string]*TokenUsage // token usage per agent (by session ID)
	activityDrillOffline bool                   // true when viewing offline agents only
	statusInvoker        *StatusInvoker         // mlld invoker for status display customization
	expandedJobClusters  map[string]bool        // job ID -> expanded (true = show workers)
	// Presence debounce state (suppress flicker from rapid presence changes)
	agentDisplayPresence map[string]types.PresenceState // presence currently being displayed
	agentActualPresence  map[string]types.PresenceState // actual presence from last poll
	agentPresenceChanged map[string]time.Time           // when actual presence last changed
	agentOrigins         map[string]map[string]struct{} // cached origins per agent for display
	// Animation frame counter for spawn cycle (incremented every activity poll)
	animationFrame int
	// New message notification state (when user has scrolled up)
	newMessageAuthors []string // authors of messages received while scrolled up
	// Peek mode state (view thread without changing posting context)
	peekThread *types.Thread    // thread being peeked (nil if not peeking)
	peekPseudo pseudoThreadKind // pseudo-thread being peeked (empty if not peeking)
	peekSource peekSourceKind   // how peek was triggered (for hint display)
	// Click debounce state (wait for possible double-click before executing single-click)
	pendingClick *pendingClick // pending single-click waiting for timeout
	// Reply reference state (for reply preview UI)
	replyToID      string // message ID being replied to (empty if no reply)
	replyToPreview string // preview text of reply target
	// Paste collapse state
	pastedText string // original pasted text when collapsed
	// Debug sync state
	debugSync       bool
	stopSyncChecker chan struct{}
	// Daemon restart detection
	daemonStartedAt int64
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

	input := newInputModel()

	vp := viewport.New(0, 0)

	messages, rawMessages, lastCursor, oldestCursor, err := loadInitialMessages(
		opts.DB,
		opts.ProjectDBPath,
		opts.Last,
		opts.IncludeArchived,
		opts.ShowUpdates,
	)
	if err != nil {
		return nil, err
	}

	lastMentionCursor := initReadWatermarks(opts.DB, opts.Username, messages, threads)

	count, err := countMessages(opts.DB, opts.IncludeArchived)
	if err != nil {
		return nil, err
	}

	model := &Model{
		db:                   opts.DB,
		projectName:          opts.ProjectName,
		projectRoot:          opts.ProjectRoot,
		projectDBPath:        opts.ProjectDBPath,
		username:             opts.Username,
		showUpdates:          opts.ShowUpdates,
		includeArchived:      opts.IncludeArchived,
		viewport:             vp,
		input:                input,
		messages:             messages,
		lastCursor:           lastCursor,
		lastMentionCursor:    lastMentionCursor,
		oldestCursor:         oldestCursor,
		status:               "",
		messageCount:         count,
		lastLimit:            opts.Last,
		hasMore:              len(rawMessages) >= opts.Last,
		colorMap:             colorMap,
		threads:              threads,
		threadIndex:          threadIndex,
		threadPanelOpen:      true,
		sidebarOpen:          false,
		visitedThreads:       make(map[string]types.Thread),
		unreadCounts:         make(map[string]int),
		collapsedThreads:     make(map[string]bool),
		favedThreads:         make(map[string]bool),
		subscribedThreads:    make(map[string]bool),
		mutedThreads:         make(map[string]bool),
		threadNicknames:      make(map[string]string),
		avatarMap:            make(map[string]string),
		agentUnreadCounts:    make(map[string]int),
		agentDisplayPresence: make(map[string]types.PresenceState),
		agentActualPresence:  make(map[string]types.PresenceState),
		agentPresenceChanged: make(map[string]time.Time),
		agentOrigins:         make(map[string]map[string]struct{}),
		expandedJobClusters:  make(map[string]bool),
		managedAgents:        managedAgents,
		agentTokenUsage:      agentTokenUsage,
		statusInvoker:        NewStatusInvoker(filepath.Join(opts.ProjectRoot, ".fray")),
		zoneManager:          zone.New(),
		channels:             channels,
		channelIndex:         channelIndex,
		initialScroll:        true,
		debugSync:            opts.DebugSync,
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
