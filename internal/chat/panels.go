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
	threadEntryMessageCollection
	threadEntryThreadCollection
	threadEntrySectionHeader // For grouping labels like "agents", "roles", "topics"
)

type messageCollectionView string

const (
	messageCollectionOpenQuestions   messageCollectionView = "open-qs"
	messageCollectionClosedQuestions messageCollectionView = "closed-qs"
	messageCollectionWondering       messageCollectionView = "wondering"
	messageCollectionStaleQuestions  messageCollectionView = "stale-qs"
)

type threadCollectionView string

const (
	threadCollectionMuted          threadCollectionView = "muted"
	threadCollectionUnreadMentions threadCollectionView = "unread-mentions"
)

type threadEntry struct {
	Kind              threadEntryKind
	Thread            *types.Thread
	MessageCollection messageCollectionView
	ThreadCollection  threadCollectionView
	Label             string
	Indent            int
	HasChildren       bool
	Collapsed         bool
	Faved             bool
	Avatar            string // Agent avatar for display in meta view
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

	header := " Channels "
	if m.sidebarFilterActive {
		if m.sidebarFilter == "" {
			header = " Channels (filter) "
		} else {
			header = fmt.Sprintf(" Channels (filter: %s) ", m.sidebarFilter)
		}
	}

	lines := []string{headerStyle.Render(header), ""} // space after header

	indices := m.sidebarMatches
	if !m.sidebarFilterActive {
		indices = make([]int, len(m.channels))
		for i := range m.channels {
			indices[i] = i
		}
	}

	if len(m.channels) == 0 {
		lines = append(lines, itemStyle.Render(" (none)"))
	} else if len(indices) == 0 {
		lines = append(lines, itemStyle.Render(" (no matches)"))
	} else {
		// Virtual scrolling: calculate visible range
		visibleHeight := m.height - 4 // header(1) + space(1) + footer(1) + padding(1)
		if visibleHeight < 1 {
			visibleHeight = 1
		}

		// Ensure scroll offset is within bounds
		if m.sidebarScrollOffset < 0 {
			m.sidebarScrollOffset = 0
		}
		maxScroll := len(indices) - visibleHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.sidebarScrollOffset > maxScroll {
			m.sidebarScrollOffset = maxScroll
		}

		// Calculate visible slice
		startIdx := m.sidebarScrollOffset
		endIdx := startIdx + visibleHeight
		if endIdx > len(indices) {
			endIdx = len(indices)
		}

		// Render only visible entries
		visibleIndices := indices[startIdx:endIdx]
		for _, index := range visibleIndices {
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
		// Close panel and focus message pane
		m.closePanels()
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
		// Check if we want to select or just close and focus message pane
		if m.sidebarFilterActive {
			return true, m.selectChannelCmd()
		}
		// Close sidebar and focus message pane
		m.closePanels()
		return true, nil
	}

	return false, nil
}

func (m *Model) startSidebarFilter() {
	if !m.sidebarFilterActive {
		m.sidebarFilterActive = true
		m.sidebarFilter = ""
		m.sidebarScrollOffset = 0
	}
	m.updateSidebarMatches()
}

func (m *Model) resetSidebarFilter() {
	m.sidebarFilterActive = false
	m.sidebarFilter = ""
	m.sidebarMatches = nil
	m.sidebarScrollOffset = 0
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
	// Sidebar has: header(1) + blank(1) = 2 lines before content
	headerLines := 2
	if line < headerLines {
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
	// Convert Y coordinate to index: subtract headers, add scroll offset
	contentLine := line - headerLines
	actualIndex := contentLine + m.sidebarScrollOffset
	if actualIndex < 0 || actualIndex >= len(indices) {
		return -1
	}
	return indices[actualIndex]
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
	entries := make([]threadEntry, 0, len(m.threads)+10)

	// Special case: viewing muted collection
	if m.viewingMutedCollection {
		// Show "back" entry for muted collection
		entries = append(entries, threadEntry{
			Kind:             threadEntryThreadCollection,
			ThreadCollection: threadCollectionMuted,
			Label:            "muted",
		})
		// Show all muted threads
		for guid := range m.mutedThreads {
			for _, thread := range m.threads {
				if thread.GUID == guid {
					t := thread
					entries = append(entries, threadEntry{
						Kind:   threadEntryThread,
						Thread: &t,
						Label:  thread.Name,
						Indent: 0,
					})
					break
				}
			}
		}
		return entries
	}

	// Build parent-child relationships
	children := make(map[string][]types.Thread)
	roots := make([]types.Thread, 0)
	for _, thread := range m.threads {
		if thread.ParentThread == nil || *thread.ParentThread == "" {
			roots = append(roots, thread)
			continue
		}
		children[*thread.ParentThread] = append(children[*thread.ParentThread], thread)
	}

	// Check if we're drilled into a thread
	drilledThread := m.currentDrillThread()
	inMetaView := false
	if drilledThread != nil {
		// When drilled in: show "back" entry + immediate children only
		entries = append(entries, threadEntry{
			Kind:   threadEntryThread,
			Thread: drilledThread,
			Label:  drilledThread.Name,
			Indent: 0,
		})
		// Filter roots to only children of drilled thread
		if kids, hasKids := children[drilledThread.GUID]; hasKids {
			roots = kids
		} else {
			roots = nil // No children
		}
		// Check if we're drilled into the meta thread
		inMetaView = drilledThread.Name == "meta" && drilledThread.ParentThread == nil
	} else {
		// At top level: show #main
		entries = append(entries, threadEntry{Kind: threadEntryMain, Label: "#main"})
	}

	// Sort children alphabetically
	for key := range children {
		slice := children[key]
		sort.Slice(slice, func(i, j int) bool {
			return slice[i].Name < slice[j].Name
		})
		children[key] = slice
	}

	// Special handling for meta view: group children into topics, agents, roles
	if inMetaView && len(roots) > 0 {
		var topics, agents, roles []types.Thread
		for _, thread := range roots {
			if strings.HasPrefix(thread.Name, "role-") {
				roles = append(roles, thread)
			} else {
				// Check if this is an agent thread (has agent-like substructure)
				_, hasNotes := children[thread.GUID]
				if hasNotes {
					agents = append(agents, thread)
				} else {
					topics = append(topics, thread)
				}
			}
		}

		// Sort each group
		sort.Slice(topics, func(i, j int) bool { return topics[i].Name < topics[j].Name })
		sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
		sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })

		// Add entries with section headers
		if len(topics) > 0 {
			entries = append(entries, threadEntry{Kind: threadEntrySectionHeader, Label: "topics"})
			for _, thread := range topics {
				t := thread
				entries = append(entries, threadEntry{
					Kind:        threadEntryThread,
					Thread:      &t,
					Label:       thread.Name,
					Indent:      0,
					HasChildren: len(children[thread.GUID]) > 0,
					Collapsed:   m.collapsedThreads[thread.GUID],
					Faved:       m.favedThreads[thread.GUID],
				})
			}
		}
		if len(agents) > 0 {
			entries = append(entries, threadEntry{Kind: threadEntrySectionHeader, Label: "agents"})
			for _, thread := range agents {
				t := thread
				avatar := m.avatarMap[thread.Name] // thread.Name is the agent_id under meta/
				entries = append(entries, threadEntry{
					Kind:        threadEntryThread,
					Thread:      &t,
					Label:       thread.Name,
					Indent:      0,
					HasChildren: len(children[thread.GUID]) > 0,
					Collapsed:   m.collapsedThreads[thread.GUID],
					Faved:       m.favedThreads[thread.GUID],
					Avatar:      avatar,
				})
			}
		}
		if len(roles) > 0 {
			entries = append(entries, threadEntry{Kind: threadEntrySectionHeader, Label: "roles"})
			for _, thread := range roles {
				t := thread
				entries = append(entries, threadEntry{
					Kind:        threadEntryThread,
					Thread:      &t,
					Label:       thread.Name,
					Indent:      0,
					HasChildren: len(children[thread.GUID]) > 0,
					Collapsed:   m.collapsedThreads[thread.GUID],
					Faved:       m.favedThreads[thread.GUID],
				})
			}
		}

		return entries
	}

	// Helper to add thread entry (no recursion - children shown via drill-in)
	var walk func(thread types.Thread, indent int)
	walk = func(thread types.Thread, indent int) {
		t := thread
		kids, hasKids := children[thread.GUID]
		faved := m.favedThreads[thread.GUID]
		entries = append(entries, threadEntry{
			Kind:        threadEntryThread,
			Thread:      &t,
			Label:       thread.Name,
			Indent:      indent,
			HasChildren: hasKids && len(kids) > 0,
			Collapsed:   false, // Not used - drill handles children
			Faved:       faved,
		})
		// No recursion - children are shown when user drills in
	}

	// Helper to check if thread has unreads with mentions
	hasUnreadMentions := func(guid string) bool {
		return false // TODO: implement mention detection
	}

	// Tracking for shown GUIDs (start early so meta is tracked)
	shownGUIDs := make(map[string]bool)

	// 1b. Meta thread always appears first (if it exists and at top level)
	if drilledThread == nil {
		for _, thread := range roots {
			if thread.Name == "meta" && (thread.ParentThread == nil || *thread.ParentThread == "") {
				t := thread
				walk(t, 0)
				shownGUIDs[thread.GUID] = true
				break
			}
		}
	}

	// 2. Threads with unread mentions/replies (special indicator)
	var unreadMentionThreads []types.Thread
	for _, thread := range roots {
		if hasUnreadMentions(thread.GUID) && !shownGUIDs[thread.GUID] {
			unreadMentionThreads = append(unreadMentionThreads, thread)
		}
	}
	for _, thread := range unreadMentionThreads {
		walk(thread, 0)
	}

	// 3. Faved threads (excluding those already shown)
	for _, t := range unreadMentionThreads {
		shownGUIDs[t.GUID] = true
	}

	var favedThreads []types.Thread
	for _, thread := range roots {
		if m.favedThreads[thread.GUID] && !shownGUIDs[thread.GUID] {
			favedThreads = append(favedThreads, thread)
		}
	}
	// Sort faved by name
	sort.Slice(favedThreads, func(i, j int) bool {
		return favedThreads[i].Name < favedThreads[j].Name
	})
	for _, thread := range favedThreads {
		walk(thread, 0)
		shownGUIDs[thread.GUID] = true
	}

	// 3b. Message collections with content (open-qs, stale-qs) - only at top level
	if m.drillDepth() == 0 {
		if m.questionCounts[pseudoThreadKind(messageCollectionOpenQuestions)] > 0 {
			entries = append(entries, threadEntry{
				Kind:              threadEntryMessageCollection,
				MessageCollection: messageCollectionOpenQuestions,
				Label:             string(messageCollectionOpenQuestions),
			})
		}
		if m.questionCounts[pseudoThreadKind(messageCollectionStaleQuestions)] > 0 {
			entries = append(entries, threadEntry{
				Kind:              threadEntryMessageCollection,
				MessageCollection: messageCollectionStaleQuestions,
				Label:             string(messageCollectionStaleQuestions),
			})
		}
	}

	// 4. Subscribed threads sorted by recency (last_activity_at)
	var subscribedThreads []types.Thread
	for _, thread := range roots {
		if m.subscribedThreads[thread.GUID] && !shownGUIDs[thread.GUID] && !m.mutedThreads[thread.GUID] {
			subscribedThreads = append(subscribedThreads, thread)
		}
	}
	sort.Slice(subscribedThreads, func(i, j int) bool {
		iActivity := subscribedThreads[i].CreatedAt
		if subscribedThreads[i].LastActivityAt != nil {
			iActivity = *subscribedThreads[i].LastActivityAt
		}
		jActivity := subscribedThreads[j].CreatedAt
		if subscribedThreads[j].LastActivityAt != nil {
			jActivity = *subscribedThreads[j].LastActivityAt
		}
		return iActivity > jActivity // most recent first
	})
	for _, thread := range subscribedThreads {
		walk(thread, 0)
		shownGUIDs[thread.GUID] = true
	}

	// 5. All other threads collapsed to top-level (non-subscribed, non-muted)
	var otherThreads []types.Thread
	for _, thread := range roots {
		if !shownGUIDs[thread.GUID] && !m.mutedThreads[thread.GUID] {
			otherThreads = append(otherThreads, thread)
		}
	}
	// Sort by name
	sort.Slice(otherThreads, func(i, j int) bool {
		return otherThreads[i].Name < otherThreads[j].Name
	})
	for _, thread := range otherThreads {
		// Calculate max depth for collapsed display
		maxDepth := m.calculateMaxDepth(thread.GUID, children, 0)
		t := thread
		entries = append(entries, threadEntry{
			Kind:        threadEntryThread,
			Thread:      &t,
			Label:       thread.Name,
			Indent:      maxDepth, // Store depth in Indent field for collapsed display
			HasChildren: false,     // Don't allow expanding in this mode
			Collapsed:   true,      // Mark as collapsed
			Faved:       false,
		})
	}

	// Muted collection at bottom - only show at top level (not when drilled in)
	if len(m.mutedThreads) > 0 && m.drillDepth() == 0 {
		entries = append(entries, threadEntry{
			Kind:             threadEntryThreadCollection,
			ThreadCollection: threadCollectionMuted,
			Label:            "muted",
		})
	}

	// Add search results from database
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

func (m *Model) calculateMaxDepth(guid string, children map[string][]types.Thread, currentDepth int) int {
	kids, hasKids := children[guid]
	if !hasKids || len(kids) == 0 {
		return currentDepth
	}
	maxDepth := currentDepth
	for _, child := range kids {
		childDepth := m.calculateMaxDepth(child.GUID, children, currentDepth+1)
		if childDepth > maxDepth {
			maxDepth = childDepth
		}
	}
	return maxDepth
}

func (m *Model) threadEntryLabel(entry threadEntry) string {
	switch entry.Kind {
	case threadEntryMain:
		// Use project name instead of "#main"
		label := m.projectName
		if m.roomUnreadCount > 0 {
			label = fmt.Sprintf("%s (%d)", label, m.roomUnreadCount)
		}
		return "   " + label // 3 space prefix for indicator column alignment
	case threadEntryThread:
		// Check if this is the "back" entry when drilled in (first entry)
		drilledThread := m.currentDrillThread()
		if drilledThread != nil && entry.Thread != nil && entry.Thread.GUID == drilledThread.GUID {
			// This is the back navigation entry - show with ❮ prefix
			return " ❮ " + entry.Label
		}

		// Check if this is a collapsed non-subscribed thread (depth stored in Indent)
		isCollapsedNonSubscribed := entry.Collapsed && entry.Thread != nil && !m.subscribedThreads[entry.Thread.GUID]

		if isCollapsedNonSubscribed {
			// For collapsed non-subscribed: name ❯❯❯ (chevrons after indicate depth)
			depthIndicator := ""
			if entry.Indent > 0 {
				depthIndicator = " " + strings.Repeat("❯", entry.Indent)
			}
			label := entry.Label + depthIndicator
			if count := m.unreadCounts[entry.Thread.GUID]; count > 0 {
				label = fmt.Sprintf("%s (%d)", label, count)
			}
			return "   " + label // 3 space prefix for indicator column alignment
		}

		// Normal thread rendering (subscribed/faved/expanded)

		// Reserve 3 chars for indicator alignment: " X " where X is indicator
		leftIndicator := "   " // default: 3 spaces

		// Check for unread mentions/replies (yellow ✦ indicator)
		hasMentions := false // TODO: implement mention detection
		unreadCount := 0
		if entry.Thread != nil {
			unreadCount = m.unreadCounts[entry.Thread.GUID]
		}

		// Priority order for left indicator:
		// 1. Yellow ✦ for unread mentions/replies (highest priority)
		// 2. Agent avatar (replaces ★ for agent threads, even when faved)
		// 3. ★ for faved threads (non-agent)
		// 4. Three spaces otherwise
		if hasMentions {
			leftIndicator = " ✦ "
		} else if entry.Avatar != "" {
			leftIndicator = " " + entry.Avatar + " "
		} else if entry.Faved {
			leftIndicator = " ★ "
		}

		// Use nickname at top level (depth 0), actual name when drilled
		displayName := entry.Label
		if entry.Thread != nil && m.drillDepth() == 0 {
			if nick, ok := m.threadNicknames[entry.Thread.GUID]; ok && nick != "" {
				displayName = nick
			}
		}

		// Build label with indentation for nested threads
		indent := strings.Repeat("  ", entry.Indent) // 2 spaces per level
		label := leftIndicator + indent + displayName

		// Add ❯ suffix for drillable items (has children)
		if entry.HasChildren {
			label += " ❯"
		}

		// Add unread count after name (only for subscribed threads)
		if entry.Thread != nil && m.subscribedThreads[entry.Thread.GUID] && unreadCount > 0 {
			label = fmt.Sprintf("%s (%d)", label, unreadCount)
		}

		return label
	case threadEntryMessageCollection:
		// Convert back to pseudoThreadKind for compatibility with questionCounts
		count := m.questionCounts[pseudoThreadKind(entry.MessageCollection)]
		if count > 0 {
			return fmt.Sprintf("   %s (%d)", entry.Label, count)
		}
		return "   " + entry.Label
	case threadEntryThreadCollection:
		// Thread collections show count of threads in collection
		if entry.ThreadCollection == threadCollectionMuted {
			count := len(m.mutedThreads)
			// When viewing muted collection, show back indicator
			if m.viewingMutedCollection {
				if count > 0 {
					return fmt.Sprintf(" ❮ %s (%d)", entry.Label, count)
				}
				return " ❮ " + entry.Label
			}
			if count > 0 {
				return fmt.Sprintf("   %s (%d)", entry.Label, count)
			}
		}
		return "   " + entry.Label
	case threadEntrySectionHeader:
		// Section headers for meta view grouping
		return " ─ " + entry.Label + " ─"
	default:
		return ""
	}
}

func (m *Model) renderThreadPanel() string {
	width := m.threadPanelWidth()
	if width <= 0 {
		return ""
	}

	// Color scheme for threads
	depthColor := m.depthColor()
	headerStyle := lipgloss.NewStyle().Foreground(depthColor).Bold(true)
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("67"))                  // dim blue
	mainStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)      // bright white bold for main
	activeStyle := lipgloss.NewStyle().Foreground(depthColor).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24")).Bold(true)
	collapsedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))            // dim grey for non-subscribed

	// Only show header when filtering or drilled in
	var lines []string
	showHeader := m.threadFilterActive || m.drillDepth() > 0
	if showHeader {
		header := ""
		if m.threadFilterActive {
			if m.threadFilter == "" {
				header = " filter: "
			} else {
				header = fmt.Sprintf(" filter: %s ", m.threadFilter)
			}
		} else if m.drillDepth() > 0 {
			// Build drill path display with depth chevrons
			depth := m.drillDepth()
			chevrons := strings.Repeat("❮", depth)
			path := ""
			if m.db != nil {
				for i, guid := range m.drillPath {
					for _, t := range m.threads {
						if t.GUID == guid {
							if i > 0 {
								path += "/"
							}
							path += t.Name
							break
						}
					}
				}
				path += "/"
			}
			header = fmt.Sprintf(" %s %s ", chevrons, path)
		}
		lines = []string{headerStyle.Render(header), ""} // space after header
	} else {
		lines = []string{""} // single blank line at top
	}

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

	// Virtual scrolling: calculate visible range
	headerLines := 1 // blank line at top
	if showHeader {
		headerLines = 2 // header + blank line
	}
	visibleHeight := m.height - headerLines - 2 // subtract header lines + footer(1) + padding(1)
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	// Adjust scroll offset to keep selection visible
	if m.threadPanelFocus {
		// Find position of selected entry in filtered list
		selectedPos := -1
		for i, idx := range indices {
			if idx == m.threadIndex {
				selectedPos = i
				break
			}
		}
		if selectedPos >= 0 {
			// Scroll down if selection is below visible area
			if selectedPos >= m.threadScrollOffset + visibleHeight {
				m.threadScrollOffset = selectedPos - visibleHeight + 1
			}
			// Scroll up if selection is above visible area
			if selectedPos < m.threadScrollOffset {
				m.threadScrollOffset = selectedPos
			}
		}
	}

	// Ensure scroll offset is within bounds
	if m.threadScrollOffset < 0 {
		m.threadScrollOffset = 0
	}
	maxScroll := len(indices) - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.threadScrollOffset > maxScroll {
		m.threadScrollOffset = maxScroll
	}

	// Calculate visible slice
	startIdx := m.threadScrollOffset
	endIdx := startIdx + visibleHeight
	if endIdx > len(indices) {
		endIdx = len(indices)
	}

	// Only render visible entries
	visibleIndices := indices[startIdx:endIdx]

	for _, index := range visibleIndices {
		entry := entries[index]
		if entry.Kind == threadEntrySeparator {
			if entry.Label == "search" {
				lines = append(lines, itemStyle.Render(" Search results:"))
			} else {
				lines = append(lines, strings.Repeat("─", width-1))
			}
			continue
		}
		if entry.Kind == threadEntrySectionHeader {
			// Section headers are dim and non-selectable
			headerLabel := m.threadEntryLabel(entry)
			lines = append(lines, collapsedStyle.Render(headerLabel))
			continue
		}
		label := m.threadEntryLabel(entry)
		wrappedLines := []string{label}
		if width > 0 {
			wrappedLines = wrapLine(label, width-1)
		}
		style := itemStyle

		// Main entry uses bold bright style
		if entry.Kind == threadEntryMain {
			style = mainStyle
		}

		// Check if this is a collapsed non-subscribed thread (for dim grey styling)
		isCollapsedNonSubscribed := entry.Kind == threadEntryThread && entry.Collapsed && entry.Thread != nil && !m.subscribedThreads[entry.Thread.GUID]
		if isCollapsedNonSubscribed {
			style = collapsedStyle
		}

		// Active highlighting (overrides collapsed styling)
		if entry.Kind == threadEntryThread && m.currentThread != nil && entry.Thread != nil && entry.Thread.GUID == m.currentThread.GUID {
			style = activeStyle
		}
		if entry.Kind == threadEntryMain && m.currentThread == nil && m.currentPseudo == "" {
			style = activeStyle
		}
		if entry.Kind == threadEntryMessageCollection && entry.MessageCollection == messageCollectionView(m.currentPseudo) {
			style = activeStyle
		}

		// Selection highlighting (overrides all other styles)
		isSelected := index == m.threadIndex && m.threadPanelFocus
		if isSelected {
			style = selectedStyle
		}

		// Append all wrapped lines
		for i, wrappedLine := range wrappedLines {
			line := wrappedLine
			if isSelected {
				// Pad to full width for selection background
				line = lipgloss.NewStyle().Width(width).Render(line)
			}
			if i == 0 {
				lines = append(lines, style.Render(line))
			} else {
				// Continuation lines (already have 2-char indent from wrapLine)
				lines = append(lines, style.Render(line))
			}
		}
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
		// If viewing muted collection, exit to top level
		if m.viewingMutedCollection {
			m.viewingMutedCollection = false
			m.threadIndex = 0
			m.threadScrollOffset = 0
			return true, nil
		}
		// If drilled in, drill out instead of closing
		if m.drillDepth() > 0 {
			m.drillOutAction()
			return true, nil
		}
		// At top level - close panel and focus message pane
		m.closePanels()
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
		m.drillOutAction()
		return true, nil
	case "l":
		m.drillInAction()
		return true, nil
	case "f":
		// /f to toggle fave (when not in filter mode)
		if !m.threadFilterActive {
			m.toggleFaveSelectedThread()
			return true, nil
		}
	}

	switch msg.Type {
	case tea.KeyUp:
		m.moveThreadSelection(-1)
		return true, nil
	case tea.KeyDown:
		m.moveThreadSelection(1)
		return true, nil
	case tea.KeyLeft:
		m.drillOutAction()
		return true, nil
	case tea.KeyRight:
		m.drillInAction()
		return true, nil
	case tea.KeyEnter:
		// Select the current thread entry
		m.selectThreadEntry()
		m.resetThreadFilter()
		return true, nil
	case tea.KeyCtrlF:
		// Ctrl-f to toggle fave
		m.toggleFaveSelectedThread()
		return true, nil
	case tea.KeyCtrlN:
		// Ctrl-n to set nickname - store target and pre-fill input
		entries := m.threadEntries()
		if m.threadIndex >= 0 && m.threadIndex < len(entries) {
			entry := entries[m.threadIndex]
			if entry.Kind == threadEntryThread && entry.Thread != nil {
				m.pendingNicknameGUID = entry.Thread.GUID
				m.input.SetValue("/n ")
				m.input.CursorEnd()
				m.threadPanelFocus = false
				m.sidebarFocus = false
				m.status = fmt.Sprintf("Enter nickname for %s (or leave empty to clear)", entry.Thread.Name)
			}
		}
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
	m.threadScrollOffset = 0
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
	// Thread panel has:
	// - When filtering or drilled: header(1) + blank(1) = 2 lines
	// - When at top level: blank(1) = 1 line
	showHeader := m.threadFilterActive || m.drillDepth() > 0
	headerLines := 1
	if showHeader {
		headerLines = 2
	}
	if line < headerLines {
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
			if entry.Kind == threadEntrySeparator || entry.Kind == threadEntrySectionHeader {
				continue
			}
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return -1
	}
	// Convert Y coordinate to index: subtract headers, add scroll offset
	contentLine := line - headerLines
	actualIndex := contentLine + m.threadScrollOffset
	if actualIndex < 0 || actualIndex >= len(indices) {
		return -1
	}
	selected := indices[actualIndex]
	if entries[selected].Kind == threadEntrySeparator || entries[selected].Kind == threadEntrySectionHeader {
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
			if entry.Kind == threadEntrySeparator || entry.Kind == threadEntrySectionHeader {
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
		// If selecting from search mode, drill sidebar to parent level (Option A)
		if m.threadFilterActive && entry.Thread != nil && entry.Thread.ParentThread != nil && *entry.Thread.ParentThread != "" {
			// Find parent thread and drill to it
			parentGUID := *entry.Thread.ParentThread
			for _, t := range m.threads {
				if t.GUID == parentGUID {
					// Build drill path to parent
					m.drillPath = make([]string, 0)
					current := &t
					path := []string{current.GUID}
					for current.ParentThread != nil && *current.ParentThread != "" {
						for _, pt := range m.threads {
							if pt.GUID == *current.ParentThread {
								path = append([]string{pt.GUID}, path...)
								current = &pt
								break
							}
						}
					}
					m.drillPath = path
					break
				}
			}
		}

		m.currentThread = entry.Thread
		m.currentPseudo = ""
		// Track visited threads for persistence in list
		if entry.Thread != nil {
			m.visitedThreads[entry.Thread.GUID] = *entry.Thread
			m.addRecentThread(*entry.Thread)
			m.markThreadAsRead(entry.Thread.GUID)
		}

		// Exit search mode after selection
		if m.threadFilterActive {
			m.resetThreadFilter()
		}
	case threadEntryMessageCollection:
		// Convert messageCollectionView back to pseudoThreadKind for compatibility
		m.currentThread = nil
		m.threadMessages = nil
		m.currentPseudo = pseudoThreadKind(entry.MessageCollection)
	case threadEntryThreadCollection:
		// Handle thread collection views
		if entry.ThreadCollection == threadCollectionMuted {
			if m.viewingMutedCollection {
				// Already viewing muted - go back to top level
				m.viewingMutedCollection = false
				m.threadIndex = 0
				m.threadScrollOffset = 0
			} else {
				// Drill into muted collection
				m.viewingMutedCollection = true
				m.threadIndex = 0
				m.threadScrollOffset = 0
			}
		}
		return
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

func (m *Model) drillDepth() int {
	return len(m.drillPath)
}

func (m *Model) currentDrillThread() *types.Thread {
	if len(m.drillPath) == 0 {
		return nil
	}
	guid := m.drillPath[len(m.drillPath)-1]
	for _, t := range m.threads {
		if t.GUID == guid {
			return &t
		}
	}
	return nil
}

func (m *Model) drillIn(thread *types.Thread) {
	if thread == nil {
		return
	}
	m.drillPath = append(m.drillPath, thread.GUID)
	m.threadIndex = 0 // Focus first child
	m.threadScrollOffset = 0
}

func (m *Model) drillOut() string {
	if len(m.drillPath) == 0 {
		return ""
	}
	// Pop last element
	lastGUID := m.drillPath[len(m.drillPath)-1]
	m.drillPath = m.drillPath[:len(m.drillPath)-1]
	m.threadScrollOffset = 0
	return lastGUID // Return the thread we were drilled into (for focus restoration)
}

func (m *Model) depthColor() lipgloss.Color {
	depth := m.drillDepth()
	switch depth {
	case 0:
		return lipgloss.Color("75") // blue
	case 1:
		return lipgloss.Color("141") // purple
	case 2:
		return lipgloss.Color("78") // green
	default:
		return lipgloss.Color("227") // yellow
	}
}

func (m *Model) drillInAction() {
	entries := m.threadEntries()
	if m.threadIndex < 0 || m.threadIndex >= len(entries) {
		return
	}
	entry := entries[m.threadIndex]
	if entry.Kind != threadEntryThread || entry.Thread == nil {
		return
	}
	// Don't drill into the same thread we're already in (prevents infinite drill on back entry)
	drilledThread := m.currentDrillThread()
	if drilledThread != nil && entry.Thread.GUID == drilledThread.GUID {
		// Already drilled into this thread - select it to view messages instead
		m.selectThreadEntry()
		return
	}
	// Check if thread has children
	var children []types.Thread
	for _, t := range m.threads {
		if t.ParentThread != nil && *t.ParentThread == entry.Thread.GUID {
			children = append(children, t)
		}
	}
	if len(children) == 0 {
		// Leaf node - just select it to view messages
		m.selectThreadEntry()
		return
	}
	// Drill in
	m.drillIn(entry.Thread)

	// Auto-select "notes" child if drilling into agent thread from meta
	// Agent threads are direct children of meta with a "notes" subthread
	if drilledThread != nil && drilledThread.Name == "meta" {
		for _, child := range children {
			if child.Name == "notes" {
				// Find and select the notes entry
				newEntries := m.threadEntries()
				for i, e := range newEntries {
					if e.Kind == threadEntryThread && e.Thread != nil && e.Thread.GUID == child.GUID {
						m.threadIndex = i
						m.selectThreadEntry()
						break
					}
				}
				break
			}
		}
	}
}

func (m *Model) toggleFaveSelectedThread() {
	if m.db == nil || m.username == "" {
		return
	}
	entries := m.threadEntries()
	if m.threadIndex < 0 || m.threadIndex >= len(entries) {
		return
	}
	entry := entries[m.threadIndex]
	if entry.Kind != threadEntryThread || entry.Thread == nil {
		return
	}

	guid := entry.Thread.GUID
	isFaved := m.favedThreads[guid]

	if isFaved {
		if err := db.RemoveFave(m.db, m.username, "thread", guid); err != nil {
			m.status = fmt.Sprintf("Error unfaving: %v", err)
			return
		}
		m.status = fmt.Sprintf("Unfaved %s", entry.Thread.Name)
	} else {
		if _, err := db.AddFave(m.db, m.username, "thread", guid); err != nil {
			m.status = fmt.Sprintf("Error faving: %v", err)
			return
		}
		m.status = fmt.Sprintf("Faved %s", entry.Thread.Name)
	}

	// Refresh faved threads
	m.refreshFavedThreads()
}

func (m *Model) drillOutAction() {
	// If viewing muted collection, exit to top level
	if m.viewingMutedCollection {
		m.viewingMutedCollection = false
		m.threadIndex = 0
		m.threadScrollOffset = 0
		// Focus the muted entry in the list
		entries := m.threadEntries()
		for i, entry := range entries {
			if entry.Kind == threadEntryThreadCollection && entry.ThreadCollection == threadCollectionMuted {
				m.threadIndex = i
				break
			}
		}
		return
	}
	if m.drillDepth() == 0 {
		return // Already at top level
	}
	returningFrom := m.drillOut()
	// Focus the thread we were drilled into
	if returningFrom != "" {
		entries := m.threadEntries()
		for i, entry := range entries {
			if entry.Kind == threadEntryThread && entry.Thread != nil && entry.Thread.GUID == returningFrom {
				m.threadIndex = i
				break
			}
		}
	}
}

func (m *Model) calculateThreadPanelWidth() {
	// Calculate width based on longest visible thread name + 4 char buffer, max 50
	maxLen := 10 // minimum width for " Threads "
	entries := m.threadEntries()
	for _, entry := range entries {
		label := m.threadEntryLabel(entry)
		// Strip ANSI codes for accurate length calculation
		cleanLabel := stripANSI(label)
		if len(cleanLabel) > maxLen {
			maxLen = len(cleanLabel)
		}
	}
	// Add 4 char buffer (1 for leading space in render + 3 for padding)
	width := maxLen + 4
	// Max 50 chars
	if width > 50 {
		width = 50
	}
	m.cachedThreadPanelWidth = width
}

func stripANSI(s string) string {
	// Simple ANSI code stripper for length calculation
	result := ""
	inEscape := false
	for _, ch := range s {
		if ch == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEscape = false
			}
			continue
		}
		result += string(ch)
	}
	return result
}

func wrapLine(value string, maxLen int) []string {
	if maxLen <= 0 {
		return []string{value}
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return []string{value}
	}
	// Wrap with 2-char indent on continuation lines
	lines := make([]string, 0)
	firstLineMax := maxLen
	contLineMax := maxLen - 2 // 2-char indent

	if contLineMax < 1 {
		// Width too small to wrap, just truncate
		return []string{string(runes[:maxLen])}
	}

	// First line
	lines = append(lines, string(runes[:firstLineMax]))
	remaining := runes[firstLineMax:]

	// Continuation lines with 2-char indent
	for len(remaining) > 0 {
		if len(remaining) <= contLineMax {
			lines = append(lines, "  "+string(remaining))
			break
		}
		lines = append(lines, "  "+string(remaining[:contLineMax]))
		remaining = remaining[contLineMax:]
	}
	return lines
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
	threads, err := db.GetThreads(dbConn, &types.ThreadQueryOptions{})
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
