package chat

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const pollInterval = time.Second
const suggestionLimit = 8
const inputMaxHeight = 6
const inputPadding = 1

var (
	agentPalette = []lipgloss.Color{
		lipgloss.Color("111"),
		lipgloss.Color("157"),
		lipgloss.Color("216"),
		lipgloss.Color("36"),
		lipgloss.Color("183"),
		lipgloss.Color("230"),
	}
	userColor   = lipgloss.Color("249")
	statusColor = lipgloss.Color("241")
	metaColor   = lipgloss.Color("242")
	inputBg     = lipgloss.Color("236")
	caretColor  = lipgloss.Color("243")
	textColor   = lipgloss.Color("255")
	blurText    = lipgloss.Color("248")
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
	db              *sql.DB
	projectName     string
	projectRoot     string
	projectDBPath   string
	username        string
	showUpdates     bool
	includeArchived bool
	viewport        viewport.Model
	input           textarea.Model
	messages        []types.Message
	lastCursor      *types.MessageCursor
	oldestCursor    *types.MessageCursor
	status          string
	width           int
	height          int
	messageCount    int
	lastLimit       int
	hasMore         bool
	colorMap        map[string]lipgloss.Color
	suggestions     []suggestionItem
	suggestionIndex int
	suggestionStart int
	suggestionKind  suggestionKind
	lastInputValue  string
	lastInputPos    int
	channels        []channelEntry
	channelIndex    int
	sidebarOpen     bool
	sidebarFocus    bool
}

type pollMsg struct {
	messages []types.Message
}

type errMsg struct {
	err error
}

type suggestionKind int

const (
	suggestionNone suggestionKind = iota
	suggestionMention
	suggestionReply
)

type suggestionItem struct {
	Display string
	Insert  string
}

type channelEntry struct {
	ID   string
	Name string
	Path string
}

// NewModel creates a chat model with recent messages loaded.
func NewModel(opts Options) (*Model, error) {
	if opts.Last <= 0 {
		opts.Last = 20
	}

	channels, channelIndex := loadChannels(opts.ProjectRoot)

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
	input.FocusedStyle.Base = lipgloss.NewStyle().Foreground(textColor).Background(inputBg)
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(textColor).Background(inputBg)
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(caretColor).Background(inputBg)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(inputBg)
	input.BlurredStyle.Base = lipgloss.NewStyle().Foreground(blurText).Background(inputBg)
	input.BlurredStyle.Text = lipgloss.NewStyle().Foreground(blurText).Background(inputBg)
	input.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(caretColor).Background(inputBg)
	input.BlurredStyle.CursorLine = lipgloss.NewStyle().Background(inputBg)
	input.Focus()

	vp := viewport.New(0, 0)

	rawMessages, err := db.GetMessages(opts.DB, &types.MessageQueryOptions{
		Limit:           opts.Last,
		IncludeArchived: opts.IncludeArchived,
	})
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
		channels:        channels,
		channelIndex:    channelIndex,
	}

	model.refreshViewport(true)
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
		if handled, cmd := m.handleSidebarKeys(msg); handled {
			return m, cmd
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
			return m, tea.Quit
		case tea.KeyEnter:
			value := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			m.clearSuggestions()
			m.lastInputValue = m.input.Value()
			m.lastInputPos = m.inputCursorPos()
			m.resize()
			if value == "" {
				return m, nil
			}
			if handled, cmd := m.handleSlashCommand(value); handled {
				return m, cmd
			}
			return m, m.handleSubmit(value)
		case tea.KeyTab:
			m.toggleSidebar()
			return m, nil
		case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			if (msg.Type == tea.KeyPgUp || msg.Type == tea.KeyHome) && m.viewport.AtTop() {
				m.loadOlderMessages()
			}
			return m, cmd
		}
		var cmd tea.Cmd
		if !m.sidebarFocus {
			m.input, cmd = m.input.Update(msg)
			m.refreshSuggestions()
			m.resize()
		}
		return m, cmd
	case tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if msg.Button == tea.MouseButtonWheelUp && m.viewport.AtTop() {
			m.loadOlderMessages()
		}
		return m, cmd
	case pollMsg:
		if len(msg.messages) > 0 {
			incoming := m.filterNewMessages(msg.messages)
			last := msg.messages[len(msg.messages)-1]
			m.lastCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}

			if len(incoming) > 0 {
				m.status = ""
				m.messages = append(m.messages, incoming...)
				for _, msg := range incoming {
					if msg.ArchivedAt == nil {
						m.messageCount++
					}
				}
				m.refreshViewport(true)
			}
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
	if !m.sidebarOpen {
		return main
	}

	sidebar := m.renderSidebar()
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
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
	if m.status == "" {
		return fmt.Sprintf("#%s", channel)
	}
	return fmt.Sprintf("%s · #%s", m.status, channel)
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
	if resolution.Kind == ReplyResolved {
		body = resolution.Body
		replyTo = &resolution.ReplyTo
	}

	agentBases, err := db.GetAgentBases(m.db)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	mentions := core.ExtractMentions(body, agentBases)

	created, err := db.CreateMessage(m.db, types.Message{
		FromAgent: m.username,
		Body:      body,
		Mentions:  mentions,
		Type:      types.MessageTypeUser,
		ReplyTo:   replyTo,
	})
	if err != nil {
		m.status = err.Error()
		return nil
	}

	if err := db.AppendMessage(m.projectDBPath, created); err != nil {
		m.status = err.Error()
		return nil
	}

	m.messages = append(m.messages, created)
	m.lastCursor = &types.MessageCursor{GUID: created.ID, TS: created.TS}
	if created.ArchivedAt == nil {
		m.messageCount++
	}
	m.status = ""
	m.refreshViewport(true)

	return nil
}

func (m *Model) handleSuggestionKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if len(m.suggestions) == 0 {
		return false, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.clearSuggestions()
		m.resize()
		return true, nil
	case tea.KeyUp:
		m.suggestionIndex--
		if m.suggestionIndex < 0 {
			m.suggestionIndex = len(m.suggestions) - 1
		}
		return true, nil
	case tea.KeyDown:
		m.suggestionIndex++
		if m.suggestionIndex >= len(m.suggestions) {
			m.suggestionIndex = 0
		}
		return true, nil
	case tea.KeyTab, tea.KeyEnter:
		if m.suggestionIndex >= 0 && m.suggestionIndex < len(m.suggestions) {
			m.applySuggestion(m.suggestions[m.suggestionIndex])
		}
		return true, nil
	}
	return false, nil
}

func (m *Model) refreshSuggestions() {
	value := m.input.Value()
	pos := m.inputCursorPos()
	if value == m.lastInputValue && pos == m.lastInputPos {
		return
	}
	m.lastInputValue = value
	m.lastInputPos = pos

	if strings.HasPrefix(strings.TrimSpace(value), "/") {
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	kind, start, prefix := findSuggestionToken(value, pos)
	if kind == suggestionNone {
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	suggestions, err := m.buildSuggestions(kind, prefix)
	if err != nil {
		m.status = err.Error()
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}
	if len(suggestions) == 0 {
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	m.suggestions = suggestions
	m.suggestionIndex = 0
	m.suggestionStart = start
	m.suggestionKind = kind
	m.resize()
}

func (m *Model) clearSuggestions() {
	m.suggestions = nil
	m.suggestionIndex = 0
	m.suggestionStart = 0
	m.suggestionKind = suggestionNone
}

func (m *Model) suggestionHeight() int {
	if len(m.suggestions) == 0 {
		return 0
	}
	return lipgloss.Height(m.renderSuggestions())
}

func (m *Model) renderSuggestions() string {
	if len(m.suggestions) == 0 {
		return ""
	}
	normalStyle := lipgloss.NewStyle().Foreground(metaColor)
	selectedStyle := lipgloss.NewStyle().Foreground(userColor).Bold(true)

	lines := make([]string, 0, len(m.suggestions))
	for i, suggestion := range m.suggestions {
		prefix := "  "
		style := normalStyle
		if i == m.suggestionIndex {
			prefix = "> "
			style = selectedStyle
		}
		line := prefix + suggestion.Display
		if m.mainWidth() > 0 {
			line = truncateLine(line, m.mainWidth())
		}
		lines = append(lines, style.Render(line))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderSidebar() string {
	width := m.sidebarWidth()
	if width <= 0 {
		return ""
	}

	headerStyle := lipgloss.NewStyle().Foreground(userColor).Bold(true)
	itemStyle := lipgloss.NewStyle().Foreground(metaColor)
	activeStyle := lipgloss.NewStyle().Foreground(userColor).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("236")).Bold(true)

	lines := []string{headerStyle.Render(" Channels ")}
	if len(m.channels) == 0 {
		lines = append(lines, itemStyle.Render(" (none)"))
	} else {
		for i, ch := range m.channels {
			label := formatChannelLabel(ch)
			line := label
			if width > 0 {
				line = truncateLine(label, width-1)
			}

			style := itemStyle
			if samePath(ch.Path, m.projectRoot) {
				style = activeStyle
			}
			if i == m.channelIndex && m.sidebarFocus {
				style = selectedStyle
			}
			lines = append(lines, style.Render(" "+line))
		}
	}

	content := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Width(width).
		Padding(0, 1).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(metaColor).
		Render(content)
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

func (m *Model) sidebarWidth() int {
	if !m.sidebarOpen {
		return 0
	}
	maxLen := len("Channels")
	for _, ch := range m.channels {
		label := formatChannelLabel(ch)
		if len(label) > maxLen {
			maxLen = len(label)
		}
	}
	width := maxLen + 4
	if width < 16 {
		width = 16
	}
	maxWidth := m.width / 2
	if maxWidth > 0 && width > maxWidth {
		width = maxWidth
	}
	return width
}

func (m *Model) mainWidth() int {
	if m.width == 0 {
		return 0
	}
	width := m.width
	if m.sidebarOpen {
		width -= m.sidebarWidth()
	}
	if width < 1 {
		width = 1
	}
	return width
}

func (m *Model) toggleSidebar() {
	if !m.sidebarOpen {
		m.sidebarOpen = true
		m.sidebarFocus = true
		m.clearSuggestions()
		m.resize()
		return
	}

	if !m.sidebarFocus {
		m.sidebarFocus = true
		m.clearSuggestions()
		m.resize()
		return
	}

	m.sidebarOpen = false
	m.sidebarFocus = false
	m.resize()
}

func (m *Model) handleSidebarKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.sidebarOpen {
		return false, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.sidebarFocus = false
		m.resize()
		return true, nil
	}

	if !m.sidebarFocus {
		return false, nil
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

func (m *Model) applySuggestion(item suggestionItem) {
	value := []rune(m.input.Value())
	cursor := m.inputCursorPos()
	start := m.suggestionStart
	if start < 0 || start > len(value) {
		start = cursor
	}
	if cursor < start {
		cursor = start
	}

	before := value[:start]
	after := value[cursor:]
	insertRunes := []rune(item.Insert)
	if len(after) == 0 {
		insertRunes = append(insertRunes, ' ')
	}

	updated := append(append(before, insertRunes...), after...)
	m.input.SetValue(string(updated))
	m.input.CursorEnd()
	m.clearSuggestions()
	m.lastInputValue = m.input.Value()
	m.lastInputPos = m.inputCursorPos()
	m.resize()
}

func (m *Model) insertInputText(text string) {
	if text == "" {
		return
	}
	m.input.InsertString(text)
	m.refreshSuggestions()
	m.resize()
}

func (m *Model) inputCursorPos() int {
	value := m.input.Value()
	if value == "" {
		return 0
	}
	lines := strings.Split(value, "\n")
	row := m.input.Line()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
	}
	col := m.input.LineInfo().ColumnOffset
	if col < 0 {
		col = 0
	}
	lineRunes := []rune(lines[row])
	if col > len(lineRunes) {
		col = len(lineRunes)
	}

	pos := 0
	for i := 0; i < row; i++ {
		pos += len([]rune(lines[i])) + 1
	}
	pos += col

	total := len([]rune(value))
	if pos > total {
		pos = total
	}
	return pos
}

func normalizeNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

func findSuggestionToken(value string, cursor int) (suggestionKind, int, string) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	for i := cursor - 1; i >= 0; i-- {
		if unicode.IsSpace(runes[i]) {
			break
		}
		if runes[i] != '@' && runes[i] != '#' {
			continue
		}
		if i > 0 && isAlphaNum(runes[i-1]) {
			return suggestionNone, 0, ""
		}
		prefix := string(runes[i+1 : cursor])
		if runes[i] == '@' {
			return suggestionMention, i, prefix
		}
		normalized := normalizePrefix(prefix)
		if len(normalized) < 2 {
			return suggestionNone, 0, ""
		}
		return suggestionReply, i, normalized
	}
	return suggestionNone, 0, ""
}

func (m *Model) buildSuggestions(kind suggestionKind, prefix string) ([]suggestionItem, error) {
	switch kind {
	case suggestionMention:
		return m.buildMentionSuggestions(prefix)
	case suggestionReply:
		return m.buildReplySuggestions(prefix)
	default:
		return nil, nil
	}
}

func (m *Model) buildMentionSuggestions(prefix string) ([]suggestionItem, error) {
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(bases)+1)
	for base := range bases {
		names = append(names, base)
	}
	if _, ok := bases["all"]; !ok {
		names = append(names, "all")
	}
	sort.Strings(names)

	normalized := strings.ToLower(prefix)
	suggestions := make([]suggestionItem, 0, suggestionLimit)
	for _, name := range names {
		if normalized != "" && !strings.HasPrefix(strings.ToLower(name), normalized) {
			continue
		}
		label := "@" + name
		suggestions = append(suggestions, suggestionItem{Display: label, Insert: label})
		if len(suggestions) >= suggestionLimit {
			break
		}
	}
	return suggestions, nil
}

func (m *Model) buildReplySuggestions(prefix string) ([]suggestionItem, error) {
	normalized := normalizePrefix(prefix)
	if len(normalized) < 2 {
		return nil, nil
	}

	rows, err := m.db.Query(`
		SELECT guid, from_agent, body
		FROM mm_messages
		WHERE guid LIKE ?
		ORDER BY ts DESC, guid DESC
		LIMIT ?
	`, fmt.Sprintf("msg-%s%%", normalized), suggestionLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	suggestions := make([]suggestionItem, 0, suggestionLimit)
	for rows.Next() {
		var guid string
		var fromAgent string
		var body string
		if err := rows.Scan(&guid, &fromAgent, &body); err != nil {
			return nil, err
		}
		displayPrefix := core.GetGUIDPrefix(guid, prefixLength)
		preview := truncatePreview(body, 40)
		display := fmt.Sprintf("#%s @%s %s", displayPrefix, fromAgent, preview)
		suggestions = append(suggestions, suggestionItem{
			Display: display,
			Insert:  "#" + guid,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return suggestions, nil
}

func (m *Model) pollCmd() tea.Cmd {
	cursor := m.lastCursor
	includeArchived := m.includeArchived
	showUpdates := m.showUpdates

	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		options := types.MessageQueryOptions{Since: cursor, IncludeArchived: includeArchived}
		messages, err := db.GetMessages(m.db, &options)
		if err != nil {
			return errMsg{err: err}
		}
		messages = filterUpdates(messages, showUpdates)
		return pollMsg{messages: messages}
	})
}

func (m *Model) refreshViewport(scrollToBottom bool) {
	content := m.renderMessages()
	m.viewport.SetContent(content)
	if scrollToBottom {
		m.viewport.GotoBottom()
	}
}

func (m *Model) loadOlderMessages() {
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

func (m *Model) renderMessages() string {
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	chunks := make([]string, 0, len(m.messages))
	for _, msg := range m.messages {
		chunks = append(chunks, m.formatMessage(msg, prefixLength))
	}
	return strings.Join(chunks, "\n\n")
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

func (m *Model) formatMessage(msg types.Message, prefixLength int) string {
	color := userColor
	if msg.Type != types.MessageTypeUser {
		color = colorForAgent(msg.FromAgent, m.colorMap)
	}

	sender := renderByline(msg.FromAgent, color)
	body := highlightCodeBlocks(msg.Body)
	width := m.mainWidth()
	if width > 0 {
		body = ansi.Wrap(body, width, "")
	}
	bodyLine := lipgloss.NewStyle().Foreground(color).Render(body)
	meta := lipgloss.NewStyle().Foreground(color).Faint(true).Render(fmt.Sprintf("#%s", core.GetGUIDPrefix(msg.ID, prefixLength)))

	lines := []string{}
	if msg.ReplyTo != nil {
		lines = append(lines, m.replyContext(*msg.ReplyTo, prefixLength))
	}
	lines = append(lines, fmt.Sprintf("%s\n%s", sender, bodyLine))
	lines = append(lines, meta)
	return strings.Join(lines, "\n")
}

func (m *Model) replyContext(replyTo string, prefixLength int) string {
	row := m.db.QueryRow(`
		SELECT from_agent, body FROM mm_messages WHERE guid = ?
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

func contrastTextColor(color lipgloss.Color) lipgloss.Color {
	code, ok := parseColorCode(color)
	if !ok {
		return lipgloss.Color("231")
	}
	r, g, b := colorCodeToRGB(code)
	luminance := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if luminance > 128 {
		return lipgloss.Color("16")
	}
	return lipgloss.Color("231")
}

func parseColorCode(color lipgloss.Color) (int, bool) {
	trimmed := strings.TrimSpace(string(color))
	if trimmed == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func colorCodeToRGB(code int) (int, int, int) {
	if code < 16 {
		standard := [16][3]int{
			{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
			{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
			{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
			{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
		}
		values := standard[code]
		return values[0], values[1], values[2]
	}

	if code >= 16 && code <= 231 {
		index := code - 16
		r := index / 36
		g := (index % 36) / 6
		b := index % 6
		toRGB := func(value int) int {
			if value == 0 {
				return 0
			}
			return 55 + value*40
		}
		return toRGB(r), toRGB(g), toRGB(b)
	}

	if code >= 232 && code <= 255 {
		gray := 8 + (code-232)*10
		return gray, gray, gray
	}

	return 128, 128, 128
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

func (m *Model) resize() {
	if m.width == 0 || m.height == 0 {
		return
	}

	width := m.mainWidth()
	inputWidth := width - inputPadding
	if inputWidth < 1 {
		inputWidth = 1
	}
	m.input.SetWidth(inputWidth)
	lineCount := m.input.LineCount()
	if lineCount < 1 {
		lineCount = 1
	}
	if lineCount > inputMaxHeight {
		lineCount = inputMaxHeight
	}
	m.input.SetHeight(lineCount)
	inputHeight := m.input.Height() + 2

	statusHeight := 1
	suggestionHeight := m.suggestionHeight()
	marginHeight := 1
	m.viewport.Width = width
	m.viewport.Height = m.height - inputHeight - statusHeight - suggestionHeight - marginHeight
	if m.viewport.Height < 1 {
		m.viewport.Height = 1
	}
	m.refreshViewport(false)
}

func buildColorMap(dbConn *sql.DB, lookback int, includeArchived bool) (map[string]lipgloss.Color, error) {
	messages, err := db.GetMessages(dbConn, &types.MessageQueryOptions{Limit: lookback, IncludeArchived: includeArchived})
	if err != nil {
		return nil, err
	}

	lastSeen := map[string]int64{}
	for _, msg := range messages {
		if msg.Type != types.MessageTypeAgent {
			continue
		}
		parsed, err := core.ParseAgentID(msg.FromAgent)
		if err != nil {
			continue
		}
		if ts, ok := lastSeen[parsed.Base]; !ok || msg.TS > ts {
			lastSeen[parsed.Base] = msg.TS
		}
	}

	ordered := make([]string, 0, len(lastSeen))
	for base := range lastSeen {
		ordered = append(ordered, base)
	}
	sortByLastSeen(ordered, lastSeen)

	colorMap := make(map[string]lipgloss.Color)
	for idx, base := range ordered {
		colorMap[base] = agentPalette[idx%len(agentPalette)]
	}
	return colorMap, nil
}

func sortByLastSeen(bases []string, lastSeen map[string]int64) {
	for i := 0; i < len(bases); i++ {
		for j := i + 1; j < len(bases); j++ {
			if lastSeen[bases[j]] > lastSeen[bases[i]] {
				bases[i], bases[j] = bases[j], bases[i]
			}
		}
	}
}

func colorForAgent(agentID string, colorMap map[string]lipgloss.Color) lipgloss.Color {
	parsed, err := core.ParseAgentID(agentID)
	base := agentID
	if err == nil {
		base = parsed.Base
	}
	if color, ok := colorMap[base]; ok {
		return color
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(base))
	idx := int(h.Sum32()) % len(agentPalette)
	return agentPalette[idx]
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
	query := "SELECT COUNT(*) FROM mm_messages"
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
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
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
		m.resize()
		return nil
	}
	if err := m.switchChannel(entry); err != nil {
		m.status = err.Error()
		return nil
	}
	m.sidebarFocus = false
	m.sidebarOpen = false
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
	dbPath := filepath.Join(rootPath, ".mm", "mm.db")
	if _, err := os.Stat(dbPath); err != nil {
		return core.Project{}, fmt.Errorf("channel database not found at %s", dbPath)
	}
	return core.Project{Root: rootPath, DBPath: dbPath}, nil
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
