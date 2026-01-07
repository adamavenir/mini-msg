package chat

import (
	"database/sql"
	"fmt"
	"os"
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

var (
	userColor     = lipgloss.Color("249")
	statusColor   = lipgloss.Color("241")
	metaColor     = lipgloss.Color("242")
	inputBg       = lipgloss.Color("236")
	caretColor    = lipgloss.Color("243")
	reactionColor = lipgloss.Color("220")
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
}

type errMsg struct {
	err error
}

// NewModel creates a chat model with recent messages loaded.
func NewModel(opts Options) (*Model, error) {
	if opts.Last <= 0 {
		opts.Last = 20
	}

	channels, channelIndex := loadChannels(opts.ProjectRoot)
	threads, threadIndex := loadThreads(opts.DB, opts.Username)

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
		hasMore:         len(rawMessages) < count,
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
		zoneManager:        zone.New(),
		channels:        channels,
		channelIndex:    channelIndex,
		initialScroll:   true,
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
	return m.pollCmd()
}

func (m *Model) Close() {
	if m.db != nil {
		_ = m.db.Close()
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
			if m.input.Value() != "" {
				m.input.Reset()
				m.clearSuggestions()
				m.lastInputValue = m.input.Value()
				m.lastInputPos = m.inputCursorPos()
				m.updateInputStyle()
				m.resize()
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyEnter:
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
				return m, cmd
			}
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
			return m, cmd
		}
		var cmd tea.Cmd
		if !m.sidebarFocus && !m.threadPanelFocus {
			m.input, cmd = m.input.Update(msg)
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
					m.refreshViewport(true)
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

		m.refreshQuestionCounts()
		m.refreshUnreadCounts()

		if err := m.refreshReactions(); err != nil {
			m.status = err.Error()
		}

		// Check for navigation request from notification click
		m.checkGotoFile()

		return m, m.pollCmd()
	case errMsg:
		m.status = msg.err.Error()
		return m, m.pollCmd()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.refreshSuggestions()
	return m, cmd
}

func (m *Model) View() string {
	statusLine := lipgloss.NewStyle().Foreground(statusColor).Render(m.statusLine())

	lines := []string{m.viewport.View()}
	if suggestions := m.renderSuggestions(); suggestions != "" {
		lines = append(lines, suggestions)
	}
	lines = append(lines, "", m.renderInput(), statusLine)

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
	content := m.input.View()
	style := lipgloss.NewStyle().Background(inputBg).Padding(0, inputPadding, 0, 0)
	if width := m.mainWidth(); width > 0 {
		style = style.Width(width)
	}
	blank := style.Render("")
	return strings.Join([]string{blank, style.Render(content), blank}, "\n")
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
	resolution, err := ResolveReplyReference(m.db, text)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if resolution.Kind == ReplyAmbiguous {
		m.status = m.ambiguousStatus(resolution)
		return nil
	}

	body := text
	var replyTo *string
	var replyMatch *ReplyMatch
	if resolution.Kind == ReplyResolved {
		body = resolution.Body
		replyTo = &resolution.ReplyTo
		replyMatch = resolution.Match
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
	mentions := core.ExtractMentions(body, agentBases)
	mentions = core.ExpandAllMention(mentions, agentBases)

	var replyMsg *types.Message
	if replyTo != nil && m.currentThread != nil {
		replyMsg, _ = db.GetMessage(m.db, *replyTo)
	}

	home := ""
	if m.currentThread != nil {
		home = m.currentThread.GUID
	}
	created, err := db.CreateMessage(m.db, types.Message{
		FromAgent: m.username,
		Body:      body,
		Mentions:  mentions,
		Type:      types.MessageTypeUser,
		ReplyTo:   replyTo,
		Home:      home,
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

	body := updated.Body
	if match != nil && match.Body != "" {
		body = match.Body
	}
	eventLine := core.FormatReactionEvent([]string{m.username}, reaction, updated.ID, body)
	m.messages = append(m.messages, newEventMessage(eventLine))
	m.status = ""
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
	threadWidth := 0
	if m.threadPanelOpen {
		threadWidth = m.threadPanelWidth()
		if msg.X < threadWidth {
			// Clicked in thread panel - give it focus
			m.threadPanelFocus = true
			m.sidebarFocus = false
			if msg.Y < lipgloss.Height(m.renderThreadPanel()) {
				if index := m.threadIndexAtLine(msg.Y); index >= 0 {
					// If clicking already-selected thread, drill in (if it has children)
					if index == m.threadIndex {
						m.drillInAction()
					} else {
						m.threadIndex = index
						m.selectThreadEntry()
					}
					return true, nil
				}
				// Click on header (line 0) when drilled in -> drill out
				if msg.Y == 0 && m.drillDepth() > 0 {
					m.drillOutAction()
					return true, nil
				}
				return true, nil
			}
			return true, nil
		}
	}

	if m.sidebarOpen {
		sidebarStart := threadWidth
		if msg.X >= sidebarStart && msg.X < sidebarStart+m.sidebarWidth() {
			// Clicked in sidebar - give it focus
			m.sidebarFocus = true
			m.threadPanelFocus = false
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

	if msg.Y >= m.viewport.Height {
		return false, nil
	}

	line := m.viewport.YOffset + msg.Y
	message, ok := m.messageAtLine(line)
	if !ok || message == nil {
		return ok, nil
	}

	now := time.Now()
	if m.lastClickID == message.ID && now.Sub(m.lastClickAt) <= doubleClickInterval {
		m.lastClickID = ""
		m.lastClickAt = time.Time{}
		m.copyFromZone(msg, *message)
		return true, nil
	}

	m.lastClickID = message.ID
	m.lastClickAt = now
	m.prefillReply(*message)
	return true, nil
}

func (m *Model) copyFromZone(mouseMsg tea.MouseMsg, msg types.Message) {
	// Check which zone was clicked
	guidZone := fmt.Sprintf("guid-%s", msg.ID)
	bylineZone := fmt.Sprintf("byline-%s", msg.ID)
	footerZone := fmt.Sprintf("footer-%s", msg.ID)

	var textToCopy string
	var description string

	// Check each zone type in priority order
	if m.zoneManager.Get(guidZone).InBounds(mouseMsg) {
		// Double-clicked on GUID - copy just the ID
		prefixLength := core.GetDisplayPrefixLength(m.messageCount)
		textToCopy = core.GetGUIDPrefix(msg.ID, prefixLength)
		description = "message ID"
	} else if m.zoneManager.Get(bylineZone).InBounds(mouseMsg) || m.zoneManager.Get(footerZone).InBounds(mouseMsg) {
		// Double-clicked on byline or footer - copy whole message
		textToCopy = msg.Body
		if msg.Type != types.MessageTypeEvent {
			textToCopy = fmt.Sprintf("@%s: %s", msg.FromAgent, msg.Body)
		}
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
				// Fallback: copy whole message
				textToCopy = msg.Body
				if msg.Type != types.MessageTypeEvent {
					textToCopy = fmt.Sprintf("@%s: %s", msg.FromAgent, msg.Body)
				}
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
	text := msg.Body
	if msg.Type != types.MessageTypeEvent {
		text = fmt.Sprintf("@%s: %s", msg.FromAgent, msg.Body)
	}
	if err := copyToClipboard(text); err != nil {
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
				agents := make([]string, 0, len(entries))
				for _, e := range entries {
					agents = append(agents, e.AgentID)
				}
				eventLine := core.FormatReactionEvent(agents, reaction, msg.ID, msg.Body)
				events = append(events, newEventMessage(eventLine))
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
