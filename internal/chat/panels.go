package chat

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	Kind        threadEntryKind
	Thread      *types.Thread
	Pseudo      pseudoThreadKind
	Label       string
	Indent      int
	HasChildren bool
	Collapsed   bool
	Faved       bool
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

	// Sort roots: faved first, then alphabetically
	sort.Slice(roots, func(i, j int) bool {
		iFaved := m.favedThreads[roots[i].GUID]
		jFaved := m.favedThreads[roots[j].GUID]
		if iFaved != jFaved {
			return iFaved // faved threads sort first
		}
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
		kids, hasKids := children[thread.GUID]
		collapsed := m.collapsedThreads[thread.GUID]
		faved := m.favedThreads[thread.GUID]
		entries = append(entries, threadEntry{
			Kind:        threadEntryThread,
			Thread:      &t,
			Label:       thread.Name,
			Indent:      indent,
			HasChildren: hasKids && len(kids) > 0,
			Collapsed:   collapsed,
			Faved:       faved,
		})
		if hasKids && !collapsed {
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
		if m.roomUnreadCount > 0 {
			return fmt.Sprintf("%s (%d)", entry.Label, m.roomUnreadCount)
		}
		return entry.Label
	case threadEntryThread:
		prefix := strings.Repeat("  ", entry.Indent)
		// Add expand/collapse indicator for threads with children
		indicator := ""
		if entry.HasChildren {
			if entry.Collapsed {
				indicator = "▸ "
			} else {
				indicator = "▾ "
			}
		}
		// Add fave indicator
		faveIndicator := ""
		if entry.Faved {
			faveIndicator = "★ "
		}
		label := prefix + indicator + faveIndicator + entry.Label
		if entry.Thread != nil {
			if count := m.unreadCounts[entry.Thread.GUID]; count > 0 {
				label = fmt.Sprintf("%s (%d)", label, count)
			}
		}
		return label
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
	case "h":
		m.collapseSelectedThread()
		return true, nil
	case "l":
		m.expandSelectedThread()
		return true, nil
	}

	switch msg.Type {
	case tea.KeyUp:
		m.moveThreadSelection(-1)
		return true, nil
	case tea.KeyDown:
		m.moveThreadSelection(1)
		return true, nil
	case tea.KeyLeft:
		m.collapseSelectedThread()
		return true, nil
	case tea.KeyRight:
		m.expandSelectedThread()
		return true, nil
	case tea.KeyEnter:
		m.selectThreadEntry()
		return true, nil
	}

	return false, nil
}

func (m *Model) collapseSelectedThread() {
	entries := m.threadEntries()
	if m.threadIndex < 0 || m.threadIndex >= len(entries) {
		return
	}
	entry := entries[m.threadIndex]
	if entry.Kind != threadEntryThread || entry.Thread == nil {
		return
	}
	if !entry.HasChildren {
		return
	}
	m.collapsedThreads[entry.Thread.GUID] = true
}

func (m *Model) expandSelectedThread() {
	entries := m.threadEntries()
	if m.threadIndex < 0 || m.threadIndex >= len(entries) {
		return
	}
	entry := entries[m.threadIndex]
	if entry.Kind != threadEntryThread || entry.Thread == nil {
		return
	}
	delete(m.collapsedThreads, entry.Thread.GUID)
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
		m.markRoomAsRead()
	case threadEntryThread:
		m.currentThread = entry.Thread
		m.currentPseudo = ""
		// Track visited threads for persistence in list
		if entry.Thread != nil {
			m.visitedThreads[entry.Thread.GUID] = *entry.Thread
			m.addRecentThread(*entry.Thread)
			m.markThreadAsRead(entry.Thread.GUID)
		}
	case threadEntryPseudo:
		m.currentPseudo = entry.Pseudo
	default:
		return
	}
	m.refreshThreadMessages()
	m.refreshPseudoQuestions()
	m.refreshQuestionCounts()
	m.refreshUnreadCounts()
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

func projectFromRoot(rootPath string) (core.Project, error) {
	dbPath := filepath.Join(rootPath, ".fray", "fray.db")
	if _, err := os.Stat(dbPath); err != nil {
		return core.Project{}, fmt.Errorf("channel database not found at %s", dbPath)
	}
	return core.Project{Root: rootPath, DBPath: dbPath}, nil
}
