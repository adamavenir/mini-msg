package chat

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	program := tea.NewProgram(model, tea.WithMouseCellMotion())
	_, err = program.Run()
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
	threads             []types.Thread
	threadIndex         int
	threadPanelOpen     bool
	threadPanelFocus    bool
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
	channels            []channelEntry
	channelIndex        int
	sidebarOpen         bool
	sidebarFocus        bool
	sidebarFilter       string
	sidebarMatches      []int
	sidebarFilterActive bool
	helpMessageID       string
	initialScroll       bool
	lastClickID         string
	lastClickAt         time.Time
}

type errMsg struct {
	err error
}

type channelEntry struct {
	ID   string
	Name string
	Path string
}

type threadEntryKind int

const (
	threadEntryMain threadEntryKind = iota
	threadEntryThread
	threadEntrySeparator
	threadEntryPseudo
)

type threadEntry struct {
	Kind   threadEntryKind
	Thread *types.Thread
	Pseudo pseudoThreadKind
	Label  string
	Indent int
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
		lastCursor:      lastCursor,
		oldestCursor:    oldestCursor,
		status:          "",
		messageCount:    count,
		lastLimit:       opts.Last,
		hasMore:         len(rawMessages) < count,
		colorMap:        colorMap,
		threads:            threads,
		threadIndex:        threadIndex,
		threadPanelOpen: true,
		sidebarOpen:     false,
		visitedThreads:  make(map[string]types.Thread),
		channels:        channels,
		channelIndex:    channelIndex,
		initialScroll:   true,
	}
	model.refreshQuestionCounts()
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
		case tea.KeyTab:
			if len(m.suggestions) > 0 {
				return m, nil
			}
			m.cyclePanelFocus()
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
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if msg.Button == tea.MouseButtonWheelUp && m.nearTop() {
			m.loadOlderMessages()
		}
		return m, cmd
	case pollMsg:
		if len(msg.roomMessages) > 0 {
			incoming := m.filterNewMessages(msg.roomMessages)
			last := msg.roomMessages[len(msg.roomMessages)-1]
			m.lastCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}

			if len(incoming) > 0 {
				m.status = ""
				m.messages = append(m.messages, incoming...)
				for _, msg := range incoming {
					if msg.ArchivedAt == nil {
						m.messageCount++
					}
				}
				if m.currentThread == nil && m.currentPseudo == "" {
					m.refreshViewport(true)
				}
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
		m.refreshQuestionCounts()

		if err := m.refreshReactions(); err != nil {
			m.status = err.Error()
		}
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
	if len(panels) == 0 {
		return main
	}
	left := lipgloss.JoinHorizontal(lipgloss.Top, panels...)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, main)
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
	channel := m.channelLabel()
	right := "? for help"
	threadLabel := m.currentThreadLabel()
	left := fmt.Sprintf("#%s · %s", channel, threadLabel)
	if m.status != "" {
		left = fmt.Sprintf("%s · #%s · %s", m.status, channel, threadLabel)
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
	updated, changed, err := db.AddReaction(m.db, messageID, m.username, reaction)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if !changed {
		m.status = "Reaction already added."
		return nil
	}

	update := db.MessageUpdateJSONLRecord{ID: updated.ID, Reactions: &updated.Reactions}
	if err := db.AppendMessageUpdate(m.projectDBPath, update); err != nil {
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
			if msg.Y < lipgloss.Height(m.renderThreadPanel()) {
				if index := m.threadIndexAtLine(msg.Y); index >= 0 {
					m.threadIndex = index
					m.selectThreadEntry()
					return true, nil
				}
				return true, nil
			}
		}
	}

	if m.sidebarOpen {
		sidebarStart := threadWidth
		if msg.X >= sidebarStart && msg.X < sidebarStart+m.sidebarWidth() {
			if msg.Y < lipgloss.Height(m.renderSidebar()) {
				if index := m.sidebarIndexAtLine(msg.Y); index >= 0 {
					m.channelIndex = index
					return true, m.selectChannelCmd()
				}
				return true, nil
			}
		}
	}

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
		m.copyMessage(*message)
		return true, nil
	}

	m.lastClickID = message.ID
	m.lastClickAt = now
	m.prefillReply(*message)
	return true, nil
}

func (m *Model) sidebarIndexAtLine(line int) int {
	if line <= 0 {
		return -1
	}
	if len(m.channels) == 0 {
		return -1
	}
	indices := m.sidebarMatches
	if !m.sidebarFilterActive {
		indices = make([]int, len(m.channels))
		for i := range m.channels {
			indices[i] = i
		}
	}
	if len(indices) == 0 {
		return -1
	}
	index := line - 1
	if index < 0 || index >= len(indices) {
		return -1
	}
	return indices[index]
}

func (m *Model) messageAtLine(line int) (*types.Message, bool) {
	if line < 0 {
		return nil, false
	}
	if m.currentPseudo != "" {
		return nil, false
	}
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	cursor := 0
	messages := m.currentMessages()
	emptyReadTo := map[string][]string{}
	for i, msg := range messages {
		formatted := m.formatMessage(msg, prefixLength, emptyReadTo)
		lines := lipgloss.Height(formatted)
		if line >= cursor && line < cursor+lines {
			if msg.Type == types.MessageTypeEvent {
				return nil, true
			}
			return &messages[i], true
		}
		cursor += lines
		if i < len(messages)-1 {
			if line == cursor {
				return nil, true
			}
			cursor++
		}
	}
	return nil, false
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

func (m *Model) renderSidebar() string {
	width := m.sidebarWidth()
	if width <= 0 {
		return ""
	}

	// White color scheme for channels
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // dim white
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("236")).Bold(true)
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)

	header := " Channels "
	if m.sidebarFilterActive {
		if m.sidebarFilter == "" {
			header = " Channels (filter) "
		} else {
			header = fmt.Sprintf(" Channels (filter: %s) ", m.sidebarFilter)
		}
	}

	lines := []string{headerStyle.Render(header), ""} // space after header
	if len(m.channels) == 0 {
		lines = append(lines, itemStyle.Render(" (none)"))
	} else {
		indices := m.sidebarMatches
		if !m.sidebarFilterActive {
			indices = make([]int, len(m.channels))
			for i := range m.channels {
				indices[i] = i
			}
		}
		if len(indices) == 0 {
			lines = append(lines, itemStyle.Render(" (no matches)"))
		}
		for _, index := range indices {
			ch := m.channels[index]
			label := formatChannelLabel(ch)
			line := label
			if width > 0 {
				line = truncateLine(label, width-1)
			}

			style := itemStyle
			if samePath(ch.Path, m.projectRoot) {
				style = activeStyle
			}
			if index == m.channelIndex && m.sidebarFocus {
				style = selectedStyle
			}
			lines = append(lines, style.Render(" "+line))
		}
	}

	// Recent threads section (up to 3)
	if len(m.recentThreads) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionStyle.Render(" Recent threads"))
		limit := 3
		if len(m.recentThreads) < limit {
			limit = len(m.recentThreads)
		}
		for i := 0; i < limit; i++ {
			t := m.recentThreads[i]
			label := t.Name
			if width > 0 {
				label = truncateLine(label, width-3)
			}
			lines = append(lines, itemStyle.Render("  "+label))
		}
	}

	if m.height > 0 {
		for len(lines) < m.height-1 {
			lines = append(lines, "")
		}
	}
	lines = append(lines, itemStyle.Render(" # - filter"))

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(width).Render(content)
}

func (m *Model) threadEntries() []threadEntry {
	entries := make([]threadEntry, 0, len(m.threads)+6)
	entries = append(entries, threadEntry{Kind: threadEntryMain, Label: "#main"})

	children := make(map[string][]types.Thread)
	roots := make([]types.Thread, 0)
	for _, thread := range m.threads {
		if thread.ParentThread == nil || *thread.ParentThread == "" {
			roots = append(roots, thread)
			continue
		}
		children[*thread.ParentThread] = append(children[*thread.ParentThread], thread)
	}

	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Name < roots[j].Name
	})
	for key := range children {
		slice := children[key]
		sort.Slice(slice, func(i, j int) bool {
			return slice[i].Name < slice[j].Name
		})
		children[key] = slice
	}

	var walk func(thread types.Thread, indent int)
	walk = func(thread types.Thread, indent int) {
		t := thread
		entries = append(entries, threadEntry{
			Kind:   threadEntryThread,
			Thread: &t,
			Label:  thread.Name,
			Indent: indent,
		})
		if kids, ok := children[thread.GUID]; ok {
			for _, child := range kids {
				walk(child, indent+1)
			}
		}
	}

	for _, thread := range roots {
		walk(thread, 0)
	}

	// Add visited threads (from search) that aren't in subscribed list
	if len(m.visitedThreads) > 0 {
		subscribed := make(map[string]struct{})
		for _, t := range m.threads {
			subscribed[t.GUID] = struct{}{}
		}
		for guid, thread := range m.visitedThreads {
			if _, ok := subscribed[guid]; ok {
				continue
			}
			t := thread
			entries = append(entries, threadEntry{
				Kind:   threadEntryThread,
				Thread: &t,
				Label:  thread.Name,
				Indent: 0,
			})
		}
	}

	// Pseudo-threads always at bottom (no separator) - only show actionable: open and stale
	entries = append(entries,
		threadEntry{Kind: threadEntryPseudo, Pseudo: pseudoThreadOpen, Label: string(pseudoThreadOpen)},
		threadEntry{Kind: threadEntryPseudo, Pseudo: pseudoThreadStale, Label: string(pseudoThreadStale)},
	)

	// Add search results from database (threads not in subscribed list)
	if len(m.threadSearchResults) > 0 {
		entries = append(entries, threadEntry{Kind: threadEntrySeparator, Label: "search"})
		for _, thread := range m.threadSearchResults {
			t := thread
			entries = append(entries, threadEntry{
				Kind:   threadEntryThread,
				Thread: &t,
				Label:  thread.Name,
				Indent: 0,
			})
		}
	}

	return entries
}

func (m *Model) threadEntryLabel(entry threadEntry) string {
	switch entry.Kind {
	case threadEntryMain:
		return entry.Label
	case threadEntryThread:
		prefix := strings.Repeat("  ", entry.Indent)
		return prefix + entry.Label
	case threadEntryPseudo:
		count := m.questionCounts[entry.Pseudo]
		if count > 0 {
			return fmt.Sprintf("%s (%d)", entry.Label, count)
		}
		return entry.Label
	default:
		return ""
	}
}

func (m *Model) renderThreadPanel() string {
	width := m.threadPanelWidth()
	if width <= 0 {
		return ""
	}

	// Blue color scheme for threads
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)  // bright blue
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("67"))               // dim blue
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)  // bright blue
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24")).Bold(true)

	header := " Threads "
	if m.threadFilterActive {
		if m.threadFilter == "" {
			header = " Threads (filter) "
		} else {
			header = fmt.Sprintf(" Threads (filter: %s) ", m.threadFilter)
		}
	}

	lines := []string{headerStyle.Render(header), ""} // space after header
	entries := m.threadEntries()
	indices := m.threadMatches
	if !m.threadFilterActive {
		indices = make([]int, len(entries))
		for i := range entries {
			indices[i] = i
		}
	}
	if len(entries) == 0 {
		lines = append(lines, itemStyle.Render(" (none)"))
	} else if len(indices) == 0 {
		lines = append(lines, itemStyle.Render(" (no matches)"))
	}

	for _, index := range indices {
		entry := entries[index]
		if entry.Kind == threadEntrySeparator {
			if entry.Label == "search" {
				lines = append(lines, itemStyle.Render(" Search results:"))
			} else {
				lines = append(lines, strings.Repeat("─", width-1))
			}
			continue
		}
		label := m.threadEntryLabel(entry)
		line := label
		if width > 0 {
			line = truncateLine(label, width-1)
		}
		style := itemStyle
		if entry.Kind == threadEntryThread && m.currentThread != nil && entry.Thread != nil && entry.Thread.GUID == m.currentThread.GUID {
			style = activeStyle
		}
		if entry.Kind == threadEntryMain && m.currentThread == nil && m.currentPseudo == "" {
			style = activeStyle
		}
		if entry.Kind == threadEntryPseudo && entry.Pseudo == m.currentPseudo {
			style = activeStyle
		}
		if index == m.threadIndex && m.threadPanelFocus {
			style = selectedStyle
		}
		lines = append(lines, style.Render(" "+line))
	}

	if m.height > 0 {
		for len(lines) < m.height-1 {
			lines = append(lines, "")
		}
	}
	lines = append(lines, itemStyle.Render(" Space - filter"))
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
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

func (m *Model) startSidebarFilter() {
	if !m.sidebarFilterActive {
		m.sidebarFilterActive = true
		m.sidebarFilter = ""
	}
	m.updateSidebarMatches()
}

func (m *Model) resetSidebarFilter() {
	m.sidebarFilterActive = false
	m.sidebarFilter = ""
	m.sidebarMatches = nil
}

func (m *Model) updateSidebarMatches() {
	if !m.sidebarFilterActive {
		m.sidebarMatches = nil
		return
	}

	term := strings.ToLower(strings.TrimSpace(m.sidebarFilter))
	matches := make([]int, 0, len(m.channels))
	for i, ch := range m.channels {
		if term == "" || channelMatchesFilter(ch, term) {
			matches = append(matches, i)
		}
	}
	m.sidebarMatches = matches
	if len(matches) > 0 {
		m.channelIndex = matches[0]
	}
}

func channelMatchesFilter(entry channelEntry, term string) bool {
	name := strings.ToLower(entry.Name)
	id := strings.ToLower(entry.ID)
	if name == "" {
		name = id
	}
	return strings.Contains(name, term) || strings.Contains(id, term)
}

func (m *Model) handleSidebarKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.sidebarOpen {
		return false, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.sidebarFilterActive {
			m.resetSidebarFilter()
			m.resize()
			return true, nil
		}
		m.sidebarFocus = false
		m.resize()
		return true, nil
	}

	if !m.sidebarFocus {
		return false, nil
	}

	if !m.sidebarFilterActive {
		if msg.Type == tea.KeySpace || (msg.Type == tea.KeyRunes && !msg.Paste && (msg.String() == "#" || msg.String() == " ")) {
			m.startSidebarFilter()
			m.resize()
			return true, nil
		}
	}

	if m.sidebarFilterActive {
		switch msg.Type {
		case tea.KeyBackspace, tea.KeyCtrlH:
			if m.sidebarFilter != "" {
				runes := []rune(m.sidebarFilter)
				m.sidebarFilter = string(runes[:len(runes)-1])
			}
			m.updateSidebarMatches()
			m.resize()
			return true, nil
		case tea.KeyEnter:
			if len(m.sidebarMatches) == 0 {
				return true, nil
			}
		case tea.KeyRunes:
			if msg.Paste || msg.String() == " " {
				return true, nil
			}
			m.sidebarFilter += string(msg.Runes)
			m.updateSidebarMatches()
			m.resize()
			return true, nil
		}
	}

	switch msg.String() {
	case "j":
		m.moveChannelSelection(1)
		return true, nil
	case "k":
		m.moveChannelSelection(-1)
		return true, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		m.moveChannelSelection(-1)
		return true, nil
	case tea.KeyDown:
		m.moveChannelSelection(1)
		return true, nil
	case tea.KeyEnter:
		return true, m.selectChannelCmd()
	}

	return false, nil
}

func (m *Model) startThreadFilter() {
	if !m.threadFilterActive {
		m.threadFilterActive = true
		m.threadFilter = ""
	}
	m.updateThreadMatches()
}

func (m *Model) resetThreadFilter() {
	m.threadFilterActive = false
	m.threadFilter = ""
	m.threadMatches = nil
	m.threadSearchResults = nil
}

func (m *Model) updateThreadMatches() {
	if !m.threadFilterActive {
		m.threadMatches = nil
		m.threadSearchResults = nil
		return
	}

	term := strings.ToLower(strings.TrimSpace(m.threadFilter))

	// Search database for threads matching the filter (if we have a term)
	m.threadSearchResults = nil
	if term != "" && m.db != nil {
		allThreads, err := db.GetThreads(m.db, &types.ThreadQueryOptions{})
		if err == nil {
			// Build set of subscribed thread GUIDs
			subscribed := make(map[string]struct{})
			for _, t := range m.threads {
				subscribed[t.GUID] = struct{}{}
			}
			// Filter to matching threads not already subscribed
			for _, t := range allThreads {
				if _, ok := subscribed[t.GUID]; ok {
					continue
				}
				if strings.Contains(strings.ToLower(t.Name), term) {
					m.threadSearchResults = append(m.threadSearchResults, t)
				}
			}
		}
	}

	entries := m.threadEntries()
	matches := make([]int, 0, len(entries))
	for i, entry := range entries {
		if entry.Kind == threadEntrySeparator {
			continue
		}
		if term == "" || threadEntryMatchesFilter(entry, term) {
			matches = append(matches, i)
		}
	}
	m.threadMatches = matches
	if len(matches) > 0 {
		m.threadIndex = matches[0]
	}
}

func threadEntryMatchesFilter(entry threadEntry, term string) bool {
	label := strings.ToLower(entry.Label)
	return strings.Contains(label, term)
}

func (m *Model) handleThreadPanelKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.threadPanelOpen {
		return false, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.threadFilterActive {
			m.resetThreadFilter()
			m.resize()
			return true, nil
		}
		m.threadPanelFocus = false
		m.resize()
		return true, nil
	}

	if !m.threadPanelFocus {
		return false, nil
	}

	if !m.threadFilterActive {
		if msg.Type == tea.KeySpace || (msg.Type == tea.KeyRunes && !msg.Paste && msg.String() == " ") {
			m.startThreadFilter()
			m.resize()
			return true, nil
		}
	}

	if m.threadFilterActive {
		switch msg.Type {
		case tea.KeyBackspace, tea.KeyCtrlH:
			if m.threadFilter != "" {
				runes := []rune(m.threadFilter)
				m.threadFilter = string(runes[:len(runes)-1])
			}
			m.updateThreadMatches()
			m.resize()
			return true, nil
		case tea.KeyEnter:
			if len(m.threadMatches) == 0 {
				return true, nil
			}
		case tea.KeyRunes:
			if msg.Paste || msg.String() == " " {
				return true, nil
			}
			m.threadFilter += string(msg.Runes)
			m.updateThreadMatches()
			m.resize()
			return true, nil
		}
	}

	switch msg.String() {
	case "j":
		m.moveThreadSelection(1)
		return true, nil
	case "k":
		m.moveThreadSelection(-1)
		return true, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		m.moveThreadSelection(-1)
		return true, nil
	case tea.KeyDown:
		m.moveThreadSelection(1)
		return true, nil
	case tea.KeyEnter:
		m.selectThreadEntry()
		return true, nil
	}

	return false, nil
}

func (m *Model) threadIndexAtLine(line int) int {
	if line <= 0 {
		return -1
	}
	entries := m.threadEntries()
	if len(entries) == 0 {
		return -1
	}
	indices := m.threadMatches
	if !m.threadFilterActive {
		indices = make([]int, 0, len(entries))
		for i, entry := range entries {
			if entry.Kind == threadEntrySeparator {
				continue
			}
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return -1
	}
	index := line - 1
	if index < 0 || index >= len(indices) {
		return -1
	}
	selected := indices[index]
	if entries[selected].Kind == threadEntrySeparator {
		return -1
	}
	return selected
}

func (m *Model) moveThreadSelection(delta int) {
	entries := m.threadEntries()
	if len(entries) == 0 {
		return
	}
	indices := m.threadMatches
	if !m.threadFilterActive {
		indices = make([]int, 0, len(entries))
		for i, entry := range entries {
			if entry.Kind == threadEntrySeparator {
				continue
			}
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return
	}

	current := 0
	for i, index := range indices {
		if index == m.threadIndex {
			current = i
			break
		}
	}
	next := current + delta
	if next < 0 {
		next = len(indices) - 1
	} else if next >= len(indices) {
		next = 0
	}
	m.threadIndex = indices[next]
}

func (m *Model) selectThreadEntry() {
	entries := m.threadEntries()
	if len(entries) == 0 {
		return
	}
	if m.threadIndex < 0 || m.threadIndex >= len(entries) {
		return
	}
	entry := entries[m.threadIndex]
	switch entry.Kind {
	case threadEntryMain:
		m.currentThread = nil
		m.currentPseudo = ""
		m.threadMessages = nil
	case threadEntryThread:
		m.currentThread = entry.Thread
		m.currentPseudo = ""
		// Track visited threads for persistence in list
		if entry.Thread != nil {
			m.visitedThreads[entry.Thread.GUID] = *entry.Thread
			m.addRecentThread(*entry.Thread)
		}
	case threadEntryPseudo:
		m.currentPseudo = entry.Pseudo
	default:
		return
	}
	m.refreshThreadMessages()
	m.refreshPseudoQuestions()
	m.refreshQuestionCounts()
	m.refreshViewport(true)
}

func (m *Model) addRecentThread(thread types.Thread) {
	// Remove if already in list
	for i, t := range m.recentThreads {
		if t.GUID == thread.GUID {
			m.recentThreads = append(m.recentThreads[:i], m.recentThreads[i+1:]...)
			break
		}
	}
	// Add to front
	m.recentThreads = append([]types.Thread{thread}, m.recentThreads...)
	// Keep max 10
	if len(m.recentThreads) > 10 {
		m.recentThreads = m.recentThreads[:10]
	}
}

func (m *Model) renderMessages() string {
	if m.currentPseudo != "" {
		return m.renderQuestions()
	}
	messages := m.currentMessages()
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)

	// Build read_to map: message_guid -> list of agents
	home := "room"
	if m.currentThread != nil {
		home = m.currentThread.GUID
	}
	readToMap := make(map[string][]string)
	if positions, err := db.GetReadToForHome(m.db, home); err == nil {
		for _, pos := range positions {
			readToMap[pos.MessageGUID] = append(readToMap[pos.MessageGUID], pos.AgentID)
		}
	}

	chunks := make([]string, 0, len(messages))
	for _, msg := range messages {
		chunks = append(chunks, m.formatMessage(msg, prefixLength, readToMap))
	}
	return strings.Join(chunks, "\n\n")
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

	updated, err := db.GetMessageReactions(m.db, ids)
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
			for reaction, users := range added {
				eventLine := core.FormatReactionEvent(users, reaction, msg.ID, msg.Body)
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

func diffReactions(before, after map[string][]string) map[string][]string {
	added := map[string][]string{}
	beforeSets := reactionSets(before)
	for reaction, users := range after {
		previous := beforeSets[reaction]
		for _, user := range users {
			if _, ok := previous[user]; ok {
				continue
			}
			added[reaction] = append(added[reaction], user)
		}
	}
	return added
}

func reactionsEqual(left, right map[string][]string) bool {
	if len(left) != len(right) {
		return false
	}
	leftSets := reactionSets(left)
	rightSets := reactionSets(right)
	if len(leftSets) != len(rightSets) {
		return false
	}
	for reaction, leftUsers := range leftSets {
		rightUsers, ok := rightSets[reaction]
		if !ok || len(leftUsers) != len(rightUsers) {
			return false
		}
		for user := range leftUsers {
			if _, ok := rightUsers[user]; !ok {
				return false
			}
		}
	}
	return true
}

func reactionSets(values map[string][]string) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(values))
	for reaction, users := range values {
		set := map[string]struct{}{}
		for _, user := range users {
			if user == "" {
				continue
			}
			set[user] = struct{}{}
		}
		out[reaction] = set
	}
	return out
}

func newEventMessage(body string) types.Message {
	return types.Message{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		TS:        time.Now().Unix(),
		FromAgent: "system",
		Body:      body,
		Type:      types.MessageTypeEvent,
	}
}

func formatReactionSummary(reactions map[string][]string) string {
	if len(reactions) == 0 {
		return ""
	}
	keys := make([]string, 0, len(reactions))
	for reaction := range reactions {
		keys = append(keys, reaction)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, reaction := range keys {
		users := uniqueSortedStrings(reactions[reaction])
		if len(users) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", reaction, strings.Join(users, ", ")))
	}
	return strings.Join(parts, " · ")
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (m *Model) formatMessage(msg types.Message, prefixLength int, readToMap map[string][]string) string {
	if msg.Type == types.MessageTypeEvent {
		body := msg.Body
		hasANSI := strings.Contains(body, "\x1b[")
		width := m.mainWidth()
		if width > 0 {
			body = ansi.Wrap(body, width, "")
		}
		if hasANSI {
			return body
		}
		style := lipgloss.NewStyle().Foreground(metaColor).Italic(true)
		return style.Render(body)
	}

	color := userColor
	if msg.Type != types.MessageTypeUser {
		color = colorForAgent(msg.FromAgent, m.colorMap)
	}

	sender := renderByline(msg.FromAgent, color)
	strippedBody := core.StripQuestionSections(msg.Body)
	body := highlightCodeBlocks(strippedBody)
	width := m.mainWidth()
	if width > 0 {
		body = ansi.Wrap(body, width, "")
	}
	bodyLine := lipgloss.NewStyle().Foreground(color).Render(body)
	editedSuffix := ""
	if msg.Edited || msg.EditCount > 0 || msg.EditedAt != nil {
		editedSuffix = " (edited)"
	}

	// Build the meta line with guid and read_to markers
	guidPart := fmt.Sprintf("#%s%s", core.GetGUIDPrefix(msg.ID, prefixLength), editedSuffix)
	readToPart := ""
	if agents, ok := readToMap[msg.ID]; ok && len(agents) > 0 {
		mentions := make([]string, len(agents))
		for i, agent := range agents {
			mentions[i] = "@" + agent
		}
		readToPart = "read_to: " + strings.Join(mentions, " ")
	}

	var meta string
	if readToPart != "" && width > 0 {
		// Right-align read_to on the same line
		guidWidth := len(guidPart)
		readWidth := len(readToPart)
		padding := width - guidWidth - readWidth
		if padding < 2 {
			padding = 2
		}
		metaText := guidPart + strings.Repeat(" ", padding) + readToPart
		meta = lipgloss.NewStyle().Foreground(color).Faint(true).Render(metaText)
	} else if readToPart != "" {
		meta = lipgloss.NewStyle().Foreground(color).Faint(true).Render(guidPart + "  " + readToPart)
	} else {
		meta = lipgloss.NewStyle().Foreground(color).Faint(true).Render(guidPart)
	}

	lines := []string{}
	if msg.ReplyTo != nil {
		lines = append(lines, m.replyContext(*msg.ReplyTo, prefixLength))
	}
	lines = append(lines, fmt.Sprintf("%s\n%s", sender, bodyLine))
	if reactionLine := formatReactionSummary(msg.Reactions); reactionLine != "" {
		line := lipgloss.NewStyle().Foreground(metaColor).Faint(true).Render(reactionLine)
		if width > 0 {
			line = ansi.Wrap(line, width, "")
		}
		lines = append(lines, line)
	}
	lines = append(lines, meta)
	return strings.Join(lines, "\n")
}

func (m *Model) replyContext(replyTo string, prefixLength int) string {
	row := m.db.QueryRow(`
		SELECT from_agent, body FROM fray_messages WHERE guid = ?
	`, replyTo)
	var fromAgent string
	var body string
	if err := row.Scan(&fromAgent, &body); err != nil {
		prefix := core.GetGUIDPrefix(replyTo, prefixLength)
		return lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("↪ Reply to #%s", prefix))
	}
	preview := truncatePreview(body, 50)
	return lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("↪ Reply to @%s: %s", fromAgent, preview))
}

func renderByline(agent string, color lipgloss.Color) string {
	content := fmt.Sprintf(" @%s: ", agent)
	textColor := contrastTextColor(color)
	style := lipgloss.NewStyle().Background(color).Foreground(textColor).Bold(true)
	return style.Render(content)
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

func filterUpdates(messages []types.Message, showUpdates bool) []types.Message {
	if showUpdates {
		return messages
	}
	filtered := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Type == types.MessageTypeEvent {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
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

func truncatePreview(body string, maxLen int) string {
	compact := strings.Join(strings.Fields(body), " ")
	if len(compact) <= maxLen {
		return compact
	}
	return compact[:maxLen-3] + "..."
}

func truncateLine(value string, maxLen int) string {
	if maxLen <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}

func loadChannels(currentRoot string) ([]channelEntry, int) {
	config, err := core.ReadGlobalConfig()
	if err != nil || config == nil || len(config.Channels) == 0 {
		return nil, 0
	}

	entries := make([]channelEntry, 0, len(config.Channels))
	for id, ref := range config.Channels {
		entries = append(entries, channelEntry{ID: id, Name: ref.Name, Path: ref.Path})
	}
	sort.Slice(entries, func(i, j int) bool {
		left := strings.ToLower(entries[i].Name)
		right := strings.ToLower(entries[j].Name)
		if left == "" {
			left = strings.ToLower(entries[i].ID)
		}
		if right == "" {
			right = strings.ToLower(entries[j].ID)
		}
		return left < right
	})

	index := 0
	for i, entry := range entries {
		if samePath(entry.Path, currentRoot) {
			index = i
			break
		}
	}
	return entries, index
}

func loadThreads(dbConn *sql.DB, username string) ([]types.Thread, int) {
	if dbConn == nil || username == "" {
		return nil, 0
	}
	threads, err := db.GetThreads(dbConn, &types.ThreadQueryOptions{
		SubscribedAgent: &username,
	})
	if err != nil {
		return nil, 0
	}
	return threads, 0
}

func threadPath(dbConn *sql.DB, thread *types.Thread) (string, error) {
	if thread == nil {
		return "", nil
	}
	names := []string{thread.Name}
	parent := thread.ParentThread
	seen := map[string]struct{}{thread.GUID: {}}
	for parent != nil && *parent != "" {
		if _, ok := seen[*parent]; ok {
			return "", fmt.Errorf("thread path loop detected")
		}
		seen[*parent] = struct{}{}
		parentThread, err := db.GetThread(dbConn, *parent)
		if err != nil {
			return "", err
		}
		if parentThread == nil {
			break
		}
		names = append([]string{parentThread.Name}, names...)
		parent = parentThread.ParentThread
	}
	return strings.Join(names, "/"), nil
}

func formatChannelLabel(entry channelEntry) string {
	name := entry.Name
	if name == "" {
		name = entry.ID
	}
	return "#" + name
}

func samePath(left, right string) bool {
	normalizedLeft, errLeft := filepath.Abs(left)
	normalizedRight, errRight := filepath.Abs(right)
	if errLeft == nil && errRight == nil {
		return normalizedLeft == normalizedRight
	}
	return left == right
}

func (m *Model) moveChannelSelection(delta int) {
	if len(m.channels) == 0 {
		return
	}
	if m.sidebarFilterActive {
		if len(m.sidebarMatches) == 0 {
			return
		}
		current := 0
		found := false
		for i, index := range m.sidebarMatches {
			if index == m.channelIndex {
				current = i
				found = true
				break
			}
		}
		if !found {
			m.channelIndex = m.sidebarMatches[0]
			return
		}
		next := current + delta
		if next < 0 {
			next = len(m.sidebarMatches) - 1
		} else if next >= len(m.sidebarMatches) {
			next = 0
		}
		m.channelIndex = m.sidebarMatches[next]
		return
	}

	index := m.channelIndex + delta
	if index < 0 {
		index = len(m.channels) - 1
	} else if index >= len(m.channels) {
		index = 0
	}
	m.channelIndex = index
}

func (m *Model) selectChannelCmd() tea.Cmd {
	if len(m.channels) == 0 {
		return nil
	}
	entry := m.channels[m.channelIndex]
	if samePath(entry.Path, m.projectRoot) {
		m.sidebarFocus = false
		m.sidebarOpen = false
		m.resetSidebarFilter()
		m.resize()
		return nil
	}
	if err := m.switchChannel(entry); err != nil {
		m.status = err.Error()
		return nil
	}
	m.sidebarFocus = false
	m.sidebarOpen = false
	m.resetSidebarFilter()
	m.resize()
	return m.pollCmd()
}

func (m *Model) switchChannel(entry channelEntry) error {
	project, err := projectFromRoot(entry.Path)
	if err != nil {
		return err
	}
	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return err
	}
	if err := db.InitSchema(dbConn); err != nil {
		_ = dbConn.Close()
		return err
	}

	if m.db != nil {
		_ = m.db.Close()
	}
	m.db = dbConn
	m.projectRoot = project.Root
	m.projectDBPath = project.DBPath
	m.projectName = filepath.Base(project.Root)

	if err := m.reloadMessages(); err != nil {
		return err
	}
	m.status = fmt.Sprintf("Switched to %s", formatChannelLabel(entry))
	return nil
}

func projectFromRoot(rootPath string) (core.Project, error) {
	dbPath := filepath.Join(rootPath, ".fray", "fray.db")
	if _, err := os.Stat(dbPath); err != nil {
		return core.Project{}, fmt.Errorf("channel database not found at %s", dbPath)
	}
	return core.Project{Root: rootPath, DBPath: dbPath}, nil
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
