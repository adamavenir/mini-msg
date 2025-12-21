package chat

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const pollInterval = time.Second
const suggestionLimit = 8

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
)

// Options configure chat.
type Options struct {
	DB              *sql.DB
	ProjectName     string
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
	program := tea.NewProgram(model)
	_, err = program.Run()
	return err
}

// Model implements the chat UI.
type Model struct {
	db              *sql.DB
	projectName     string
	projectDBPath   string
	username        string
	showUpdates     bool
	includeArchived bool
	viewport        viewport.Model
	input           textinput.Model
	messages        []types.Message
	lastCursor      *types.MessageCursor
	status          string
	width           int
	height          int
	messageCount    int
	colorMap        map[string]lipgloss.Color
	suggestions     []suggestionItem
	suggestionIndex int
	suggestionStart int
	suggestionKind  suggestionKind
	lastInputValue  string
	lastInputPos    int
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

// NewModel creates a chat model with recent messages loaded.
func NewModel(opts Options) (*Model, error) {
	if opts.Last <= 0 {
		opts.Last = 20
	}

	colorMap, err := buildColorMap(opts.DB, 50, opts.IncludeArchived)
	if err != nil {
		return nil, err
	}

	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 0
	input.Focus()

	vp := viewport.New(0, 0)

	messages, err := db.GetMessages(opts.DB, &types.MessageQueryOptions{
		Limit:           opts.Last,
		IncludeArchived: opts.IncludeArchived,
	})
	if err != nil {
		return nil, err
	}

	var lastCursor *types.MessageCursor
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		lastCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}
	}

	count, err := countMessages(opts.DB)
	if err != nil {
		return nil, err
	}

	model := &Model{
		db:              opts.DB,
		projectName:     opts.ProjectName,
		projectDBPath:   opts.ProjectDBPath,
		username:        opts.Username,
		showUpdates:     opts.ShowUpdates,
		includeArchived: opts.IncludeArchived,
		viewport:        vp,
		input:           input,
		messages:        filterUpdates(messages, opts.ShowUpdates),
		lastCursor:      lastCursor,
		status:          "",
		messageCount:    count,
		colorMap:        colorMap,
	}

	model.refreshViewport(true)
	return model, nil
}

func (m *Model) Init() tea.Cmd {
	return m.pollCmd()
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
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			value := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			m.clearSuggestions()
			m.lastInputValue = m.input.Value()
			m.lastInputPos = m.input.Position()
			if value == "" {
				return m, nil
			}
			if value == "/quit" || value == "/exit" {
				return m, tea.Quit
			}
			return m, m.handleSubmit(value)
		case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.refreshSuggestions()
		return m, cmd
	case pollMsg:
		if len(msg.messages) > 0 {
			m.status = ""
			m.messages = append(m.messages, msg.messages...)
			for _, msg := range msg.messages {
				if msg.ArchivedAt == nil {
					m.messageCount++
				}
			}
			last := msg.messages[len(msg.messages)-1]
			m.lastCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}
			m.refreshViewport(true)
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
	statusLine := lipgloss.NewStyle().Foreground(statusColor).Render(" ")
	if m.status != "" {
		statusLine = lipgloss.NewStyle().Foreground(statusColor).Render(m.status)
	}

	lines := []string{m.viewport.View()}
	if suggestions := m.renderSuggestions(); suggestions != "" {
		lines = append(lines, suggestions)
	}
	lines = append(lines, statusLine, m.input.View())
	return lipgloss.JoinVertical(
		lipgloss.Left,
		lines...,
	)
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
	pos := m.input.Position()
	if value == m.lastInputValue && pos == m.lastInputPos {
		return
	}
	m.lastInputValue = value
	m.lastInputPos = pos

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
		if m.width > 0 {
			line = truncateLine(line, m.width)
		}
		lines = append(lines, style.Render(line))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) applySuggestion(item suggestionItem) {
	value := []rune(m.input.Value())
	cursor := m.input.Position()
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
	m.input.SetCursor(len(before) + len(insertRunes))
	m.clearSuggestions()
	m.lastInputValue = m.input.Value()
	m.lastInputPos = m.input.Position()
	m.resize()
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

func (m *Model) renderMessages() string {
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	chunks := make([]string, 0, len(m.messages))
	for _, msg := range m.messages {
		chunks = append(chunks, m.formatMessage(msg, prefixLength))
	}
	return strings.Join(chunks, "\n\n")
}

func (m *Model) formatMessage(msg types.Message, prefixLength int) string {
	sender := m.renderSender(msg)
	meta := lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("#%s %s", m.projectName, core.GetGUIDPrefix(msg.ID, prefixLength)))
	body := msg.Body

	lines := []string{}
	if msg.ReplyTo != nil {
		lines = append(lines, m.replyContext(*msg.ReplyTo, prefixLength))
	}
	lines = append(lines, fmt.Sprintf("%s %s", sender, body))
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

func (m *Model) renderSender(msg types.Message) string {
	color := userColor
	if msg.Type != types.MessageTypeUser {
		color = colorForAgent(msg.FromAgent, m.colorMap)
	}
	style := lipgloss.NewStyle().Foreground(color).Bold(true)
	return style.Render("@" + msg.FromAgent + ":")
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
	inputHeight := lipgloss.Height(m.input.View())
	statusHeight := 1
	suggestionHeight := m.suggestionHeight()
	m.viewport.Width = m.width
	m.viewport.Height = m.height - inputHeight - statusHeight - suggestionHeight
	if m.viewport.Height < 1 {
		m.viewport.Height = 1
	}
	m.input.Width = m.width
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

func countMessages(dbConn *sql.DB) (int, error) {
	row := dbConn.QueryRow(`
		SELECT COUNT(*) FROM mm_messages WHERE archived_at IS NULL
	`)
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

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
