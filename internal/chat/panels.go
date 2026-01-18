package chat

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

	// Add blank lines at top to match pinned permissions height (keeps panels aligned with main)
	var lines []string
	for i := 0; i < m.pinnedPermissionsHeight(); i++ {
		lines = append(lines, "")
	}
	lines = append(lines, headerStyle.Render(header), "") // space after header

	indices := m.sidebarMatches
	if !m.sidebarFilterActive {
		indices = make([]int, len(m.channels))
		for i := range m.channels {
			indices[i] = i
		}
	}

	// Use panelHeight to account for pinned content at top
	panelH := m.panelHeight()

	if len(m.channels) == 0 {
		lines = append(lines, itemStyle.Render(" (none)"))
	} else if len(indices) == 0 {
		lines = append(lines, itemStyle.Render(" (no matches)"))
	} else {
		// Virtual scrolling: calculate visible range
		visibleHeight := panelH - 4 // header(1) + space(1) + footer(1) + padding(1)
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

	// Pad to full height (m.height) since we added top padding for pinned content
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
	// Account for top padding for pinned permissions
	pinnedHeight := m.pinnedPermissionsHeight()
	if line < pinnedHeight {
		return -1
	}
	line -= pinnedHeight

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

	// Reset thread navigation state - threads from old channel don't exist here
	m.currentThread = nil
	m.currentPseudo = ""
	m.threadMessages = nil
	m.drillPath = nil
	m.threadScrollOffset = 0

	// Reset edit mode if active
	if m.editingMessageID != "" {
		m.exitEditMode()
	}

	// Clear pending input (user was composing for old channel)
	if m.input.Value() != "" {
		m.input.Reset()
		m.clearSuggestions()
		m.lastInputValue = ""
		m.lastInputPos = 0
	}

	// Reload threads for new channel
	threads, threadIndex := loadThreads(dbConn, m.username)
	m.threads = threads
	m.threadIndex = threadIndex

	// Refresh thread-related state
	m.refreshUnreadCounts()
	m.refreshFavedThreads()
	m.refreshSubscribedThreads()
	m.refreshMutedThreads()
	m.refreshThreadNicknames()
	m.refreshAvatars()
	m.refreshQuestionCounts()

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

	// Color scheme depends on focus state
	depthColor := m.depthColor()
	isFocused := m.threadPanelFocus
	isPeeking := m.isPeeking()

	// When focused OR peeking: colored text, yellow selection bar
	// When unfocused and not peeking: grey text, blue bar only on current item
	var headerStyle, itemStyle, mainStyle, activeStyle, selectedStyle, collapsedStyle, currentStyle lipgloss.Style

	if isFocused || isPeeking {
		// Focused or peeking: vibrant colors, yellow selection
		headerStyle = lipgloss.NewStyle().Foreground(depthColor).Bold(true)
		itemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("67"))             // dim blue
		mainStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true) // bright white bold
		activeStyle = lipgloss.NewStyle().Foreground(depthColor).Bold(true)
		selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("220")).Bold(true) // yellow bg, black text
		collapsedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))                                             // dim grey
		currentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24")).Bold(true)  // blue bg for current
	} else {
		// Unfocused and not peeking: muted grey tones, blue bar only on current item
		headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))           // grey
		itemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))             // dim grey
		mainStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))             // bright grey
		activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))           // grey
		selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))         // just brighter, no bg
		collapsedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))        // very dim grey
		currentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24")).Bold(true) // blue bg for current
	}

	// Only show header when filtering or drilled in
	filterHintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // dim grey

	// Add blank lines at top to match pinned permissions height (keeps panels aligned with main)
	var lines []string
	for i := 0; i < m.pinnedPermissionsHeight(); i++ {
		lines = append(lines, "")
	}

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
		lines = append(lines, headerStyle.Render(header), "") // space after header
	} else {
		// Show filter hint at top (dim grey, indented), then blank line
		lines = append(lines, filterHintStyle.Render("   <space> to filter"), "")
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

	// Calculate activity section height (for reserving space at bottom)
	activityLines, activityHeight := m.renderActivitySection(width)

	// Use panelHeight to account for pinned content at top
	panelH := m.panelHeight()

	// Virtual scrolling: calculate visible range
	// Header is always 2 lines: either header+blank or filter-hint+blank
	headerLines := 2
	// Reserve space: header lines + footer(1) + padding(1) + activity section
	visibleHeight := panelH - headerLines - 2 - activityHeight
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

		// Check if this is the current item (what's displayed in message panel)
		isCurrent := false
		if entry.Kind == threadEntryThread && m.currentThread != nil && entry.Thread != nil && entry.Thread.GUID == m.currentThread.GUID {
			isCurrent = true
		}
		if entry.Kind == threadEntryMain && m.currentThread == nil && m.currentPseudo == "" {
			isCurrent = true
		}
		if entry.Kind == threadEntryMessageCollection && entry.MessageCollection == messageCollectionView(m.currentPseudo) {
			isCurrent = true
		}

		// Apply styling based on state
		// Priority: selection > current > active > base
		isSelected := index == m.threadIndex && (isFocused || isPeeking)
		needsFullWidthBg := false

		if isSelected && isCurrent {
			// Both selected and current: use selected style (yellow when focused)
			style = selectedStyle
			needsFullWidthBg = true
		} else if isSelected {
			// Just selected (j/k cursor): yellow bar when focused
			style = selectedStyle
			needsFullWidthBg = true
		} else if isCurrent {
			// Current item (what's in message panel): blue bar always
			style = currentStyle
			needsFullWidthBg = true
		} else if !isCollapsedNonSubscribed {
			// Active styling for subscribed items
			style = activeStyle
		}

		// Append all wrapped lines
		for i, wrappedLine := range wrappedLines {
			line := wrappedLine
			if needsFullWidthBg {
				// Pad to width-3 for background color (3-char margin on right, matching left)
				bgWidth := width - 3
				if bgWidth < 1 {
					bgWidth = 1
				}
				line = lipgloss.NewStyle().Width(bgWidth).Render(line)
			}
			if i == 0 {
				lines = append(lines, style.Render(line))
			} else {
				// Continuation lines (already have 2-char indent from wrapLine)
				lines = append(lines, style.Render(line))
			}
		}
	}

	// Pad to fill space before activity section
	// Use m.height since we added top padding for pinned content
	targetHeight := m.height - activityHeight
	if targetHeight > 0 {
		for len(lines) < targetHeight {
			lines = append(lines, "")
		}
	}

	// Add activity section at bottom
	lines = append(lines, activityLines...)

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
		m.peekThreadEntry(peekSourceKeyboard)
		return true, nil
	case "k":
		m.moveThreadSelection(-1)
		m.peekThreadEntry(peekSourceKeyboard)
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
		m.peekThreadEntry(peekSourceKeyboard)
		return true, nil
	case tea.KeyDown:
		m.moveThreadSelection(1)
		m.peekThreadEntry(peekSourceKeyboard)
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
	// Account for top padding for pinned permissions
	pinnedHeight := m.pinnedPermissionsHeight()
	if line < pinnedHeight {
		return -1
	}
	line -= pinnedHeight

	// Thread panel always has 2 header lines:
	// - When filtering or drilled: header(1) + blank(1) = 2 lines
	// - When at top level: filter hint(1) + blank(1) = 2 lines
	headerLines := 2
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
	// Clear any peek state since we're actually selecting
	m.clearPeek()

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
		m.pendingScrollBottom = true // scroll to bottom when returning to main
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
		m.pendingScrollBottom = true // ensure scroll to bottom on thread switch
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
		m.pendingScrollBottom = true // scroll to bottom when viewing message collection
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
	m.resize() // Recalculate layout after clearing peek
	m.refreshViewport(true)
}

// peekThreadEntry sets peek mode to view the selected thread without changing posting context.
// This is triggered by j/k navigation and single-click.
func (m *Model) peekThreadEntry(source peekSourceKind) {
	entries := m.threadEntries()
	if len(entries) == 0 {
		return
	}
	if m.threadIndex < 0 || m.threadIndex >= len(entries) {
		return
	}
	entry := entries[m.threadIndex]

	// Check if peeking the same thing we're currently in (no peek needed)
	switch entry.Kind {
	case threadEntryMain:
		if m.currentThread == nil && m.currentPseudo == "" {
			m.clearPeek()
			m.resize() // Recalculate layout after clearing peek
			return
		}
		m.peekThread = nil
		m.peekPseudo = ""
		m.peekSource = source
	case threadEntryThread:
		if entry.Thread == nil {
			return
		}
		// Check if this is the current thread (no peek needed)
		if m.currentThread != nil && m.currentThread.GUID == entry.Thread.GUID {
			m.clearPeek()
			m.resize() // Recalculate layout after clearing peek
			return
		}
		m.peekThread = entry.Thread
		m.peekPseudo = ""
		m.peekSource = source
	case threadEntryMessageCollection:
		pseudo := pseudoThreadKind(entry.MessageCollection)
		if m.currentPseudo == pseudo {
			m.clearPeek()
			m.resize() // Recalculate layout after clearing peek
			return
		}
		m.peekThread = nil
		m.peekPseudo = pseudo
		m.peekSource = source
	case threadEntryThreadCollection:
		// Thread collections don't support peek - just navigate
		return
	default:
		return
	}
	m.resize()               // Recalculate viewport height for peek statuslines
	m.refreshViewport(true)  // Scroll to bottom when peeking new content
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

// renderActivitySection renders the activity panel section showing managed agents.
// Returns the rendered lines and the number of lines used.
func (m *Model) renderActivitySection(width int) ([]string, int) {
	if len(m.managedAgents) == 0 {
		return nil, 0
	}

	var lines []string
	recentOfflineThreshold := 4 * 60 * 60  // 4 hours in seconds (for recently offline agents)
	forkIdleThreshold := 5 * 60            // 5 minutes for fork sessions
	now := int64(0)
	if t := time.Now().Unix(); t > 0 {
		now = t
	}

	// Categorize agents into active/idle/offline, separating job workers
	var activeAgents, idleAgents, recentlyOfflineAgents []types.Agent
	offlineCount := 0                            // count of agents offline > 4h (hidden individually but shown in summary)
	jobWorkers := make(map[string][]types.Agent) // job_id -> workers

	for _, agent := range m.managedAgents {
		// Fork sessions (SessionMode is 3-char prefix, not "n" or "") hide after 5m idle
		isForkSession := agent.SessionMode != "" && agent.SessionMode != "n"
		if isForkSession {
			timeSinceActive := now - agent.LastSeen
			if agent.Presence == types.PresenceIdle || agent.Presence == types.PresenceOffline {
				if timeSinceActive > int64(forkIdleThreshold) {
					// Fork session idle > 5m: don't show in activity panel
					continue
				}
			}
		}

		// Determine if this is a job worker
		isJobWorker := agent.JobID != nil && *agent.JobID != ""

		// Categorize by presence state
		// Active: spawning, prompting, prompted, active, error
		// Idle: idle presence (has active session but idle)
		// Recently offline: offline within 4h
		// Offline: offline beyond 4h (hidden)
		category := ""
		if agent.Presence == types.PresenceSpawning || agent.Presence == types.PresencePrompting ||
			agent.Presence == types.PresencePrompted || agent.Presence == types.PresenceCompacting ||
			agent.Presence == types.PresenceActive || agent.Presence == types.PresenceError {
			category = "active"
		} else if agent.Presence == types.PresenceIdle {
			category = "idle"
		} else if agent.Presence == types.PresenceOffline {
			timeSinceActive := now - agent.LastSeen
			if timeSinceActive <= int64(recentOfflineThreshold) {
				category = "recently-offline"
			} else {
				category = "offline" // hidden
			}
		}

		if isJobWorker {
			jobWorkers[*agent.JobID] = append(jobWorkers[*agent.JobID], agent)
		} else if category == "active" {
			activeAgents = append(activeAgents, agent)
		} else if category == "idle" {
			idleAgents = append(idleAgents, agent)
		} else if category == "recently-offline" {
			recentlyOfflineAgents = append(recentlyOfflineAgents, agent)
		} else if category == "offline" {
			offlineCount++
		}
	}

	// Render regular active agents
	for _, agent := range activeAgents {
		line := m.renderAgentRow(agent, width)
		lines = append(lines, line)
	}

	// Render job worker clusters
	for jobID, workers := range jobWorkers {
		if m.expandedJobClusters[jobID] {
			// Expanded: show all workers
			for _, worker := range workers {
				line := m.renderAgentRow(worker, width)
				// Add indent for expanded workers
				lines = append(lines, "  "+line)
			}
		} else {
			// Collapsed: show cluster summary
			line := m.renderJobClusterRow(jobID, workers, width)
			lines = append(lines, line)
		}
	}

	// Render idle agents (have session but idle)
	for _, agent := range idleAgents {
		line := m.renderAgentRow(agent, width)
		lines = append(lines, line)
	}

	// Render recently offline agents (offline within 4h) with distinct visual
	for _, agent := range recentlyOfflineAgents {
		line := m.renderAgentRow(agent, width)
		// Apply italic grey styling to distinguish from idle
		styledLine := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true).Render(line)
		lines = append(lines, styledLine)
	}

	// Render offline summary (agents offline > 4h)
	if offlineCount > 0 {
		offlineLabel := fmt.Sprintf(" · %d offline", offlineCount)
		if width > 0 && len(offlineLabel) > width-1 {
			offlineLabel = offlineLabel[:width-1]
		}
		offlineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		lines = append(lines, offlineStyle.Render(offlineLabel))
	}

	return lines, len(lines)
}

// renderJobClusterRow renders a collapsed job worker cluster row.
// Format: "▶ baseAgent × count [suffix]"
func (m *Model) renderJobClusterRow(jobID string, workers []types.Agent, width int) string {
	if width <= 0 {
		width = 30
	}
	rowWidth := width - 3

	// Extract base agent name and suffix from first worker
	baseAgent := ""
	if len(workers) > 0 && workers[0].JobID != nil {
		// Worker ID format: baseAgent[suffix-idx]
		// Parse to extract base agent name
		workerID := workers[0].AgentID
		if idx := strings.Index(workerID, "["); idx > 0 {
			baseAgent = workerID[:idx]
		} else {
			baseAgent = workerID
		}
	}

	// Extract 4-char suffix from job ID (job-abc12345 -> abc1)
	suffix := ""
	if len(jobID) >= 8 {
		suffix = jobID[4:8]
	}

	// Count active vs total workers
	activeCount := 0
	for _, w := range workers {
		if w.Presence == types.PresenceActive || w.Presence == types.PresenceSpawning ||
			w.Presence == types.PresencePrompting || w.Presence == types.PresencePrompted ||
			w.Presence == types.PresenceCompacting {
			activeCount++
		}
	}

	// Determine dominant presence icon
	icon := "▶"
	if activeCount == 0 {
		icon = "▷" // all idle/offline
	}

	// Build label: "▶ baseAgent × count [suffix]"
	label := fmt.Sprintf(" %s %s × %d [%s]", icon, baseAgent, len(workers), suffix)

	// Truncate if needed
	if len(label) > rowWidth {
		label = label[:rowWidth]
	}

	// Style with agent color
	agentColor := m.colorForAgent(baseAgent)
	style := lipgloss.NewStyle().Foreground(agentColor).Bold(true)

	// Wrap in bubblezone for click handling
	zoneID := "job-cluster-" + jobID
	return m.zoneManager.Mark(zoneID, style.Render(label))
}

// renderAgentRow renders a single agent row with token usage progress bar.
// Design:
// - Icon + agent name: bold, in agent's color
// - Status: italic, dim grey
// - Background: black (used/progress) | sidebar default (unused)
// - Danger zone (>80%): red (used) | sidebar default (unused), white text
// - Offline: no background, grey icon + agent name
func (m *Model) renderAgentRow(agent types.Agent, width int) string {
	if width <= 0 {
		width = 30
	}
	rowWidth := width - 3 // extra padding from sidebar edge

	// Use debounced display presence to suppress flicker from rapid state changes
	displayPresence := agent.Presence
	if dp, ok := m.agentDisplayPresence[agent.AgentID]; ok {
		displayPresence = dp
	}

	// Status icon based on (debounced) presence
	// Spawn cycle: △ ▲ animate slowly (1.5s cycle = 6 frames at 250ms each, 3 frames per icon)
	spawnCycleIcons := []string{"△", "△", "△", "▲", "▲", "▲"}
	// Compact cycle: ◁ ◀ animate slowly (same timing)
	compactCycleIcons := []string{"◁", "◁", "◁", "◀", "◀", "◀"}
	icon := "▶"
	switch displayPresence {
	case types.PresenceActive:
		icon = "▶"
	case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted:
		icon = spawnCycleIcons[m.animationFrame%len(spawnCycleIcons)]
	case types.PresenceCompacting:
		icon = compactCycleIcons[m.animationFrame%len(compactCycleIcons)]
	case types.PresenceIdle:
		icon = "▷"
	case types.PresenceError:
		icon = "𝘅"
	case types.PresenceOffline:
		icon = "▽"
	case types.PresenceBRB:
		icon = "◁" // BRB - will respawn immediately
	}

	// Build plain text content: " icon name status (unread)"
	name := agent.AgentID
	status := ""
	if agent.Status != nil && *agent.Status != "" {
		status = *agent.Status
	}

	// Override icon based on status prefix for idle agents
	// This adds semantic meaning on top of activity-based presence
	if displayPresence == types.PresenceIdle && status != "" {
		statusLower := strings.ToLower(status)
		if strings.HasPrefix(statusLower, "awaiting:") || strings.HasPrefix(statusLower, "waiting:") {
			icon = "⧗"
		} else if strings.HasPrefix(statusLower, "done:") || strings.HasPrefix(statusLower, "complete:") {
			icon = "✓"
		} else if strings.HasPrefix(statusLower, "blocked:") || strings.HasPrefix(statusLower, "stuck:") {
			icon = "⚠"
		}
	}

	// Get StatusDisplay overrides from status.mld (if available)
	var statusDisplay *StatusDisplay
	if m.statusInvoker != nil && status != "" {
		statusDisplay = m.statusInvoker.GetDisplay(status)
	}

	// Apply status.mld icon override if present
	if statusDisplay != nil && statusDisplay.Icon != nil {
		icon = *statusDisplay.Icon
	}

	// Apply status.mld message transform if present
	displayStatus := status
	if statusDisplay != nil && statusDisplay.Message != nil {
		displayStatus = *statusDisplay.Message
	}

	unread := m.agentUnreadCounts[agent.AgentID]

	// Build the visible text content in parts for styling
	// Icon part: " icon" (gets presence-based color)
	iconPart := fmt.Sprintf(" %s", icon)
	// Name part: " name" (gets agent color)
	// Fork sessions still show #XXX suffix, but new sessions ("n") are indicated by icon color instead
	namePart := fmt.Sprintf(" %s", name)
	isForkSession := agent.SessionMode != "" && agent.SessionMode != "n"
	if isForkSession {
		namePart = fmt.Sprintf(" %s#%s", name, agent.SessionMode)
	}
	// Italic part: " status (unread)" + padding
	italicPart := ""
	if displayStatus != "" {
		italicPart += " " + displayStatus
	}
	if unread > 0 {
		italicPart += fmt.Sprintf(" (%d)", unread)
	}

	// Track positions for styling splits
	iconEndPos := len([]rune(iconPart))
	boldEndPos := iconEndPos + len([]rune(namePart)) // icon + name are both bold

	// Combine for length calculation
	content := iconPart + namePart + italicPart
	contentRunes := []rune(content)

	// Truncate content to fit row width
	if len(contentRunes) > rowWidth {
		contentRunes = contentRunes[:rowWidth]
		// Adjust boldEndPos if truncation cut into bold part
		if boldEndPos > rowWidth {
			boldEndPos = rowWidth
		}
	}
	// Pad to full row width
	for len(contentRunes) < rowWidth {
		contentRunes = append(contentRunes, ' ')
	}
	paddedContent := string(contentRunes)

	// Get agent color
	agentColor := m.colorForAgent(agent.AgentID)

	// Presence-based icon color (uses debounced display presence)
	// New sessions (SessionMode == "n") show light blue during spawn, resumed show yellow
	isNewSession := agent.SessionMode == "n"
	var iconColor lipgloss.Color
	switch displayPresence {
	case types.PresenceSpawning, types.PresencePrompting, types.PresencePrompted:
		if isNewSession {
			iconColor = lipgloss.Color("117") // light blue - new session spinning up
		} else {
			iconColor = lipgloss.Color("226") // bright yellow - resumed session spinning up
		}
	case types.PresenceCompacting:
		iconColor = lipgloss.Color("226") // bright yellow - compacting context
	case types.PresenceActive:
		iconColor = lipgloss.Color("46") // bright green - active
	case types.PresenceIdle:
		iconColor = lipgloss.Color("250") // dim white - idle
	case types.PresenceOffline:
		iconColor = lipgloss.Color("250") // dim white - offline
	case types.PresenceError:
		iconColor = lipgloss.Color("196") // red - error
	case types.PresenceBRB:
		iconColor = lipgloss.Color("226") // bright yellow - will respawn
	default:
		iconColor = lipgloss.Color("240") // gray fallback
	}

	// Calculate token usage percentage
	// Target: 80% of 200k = 160k tokens is "full"
	// Danger zone: >80% actual (>160k) switches to red
	const maxTokens = 200000
	const targetPercent = 0.80 // 80% is "full" (160k tokens)
	const dangerThreshold = 0.80

	tokenPercent := 0.0
	if usage := m.agentTokenUsage[agent.AgentID]; usage != nil {
		contextTokens := usage.ContextTokens()
		if contextTokens > 0 {
			tokenPercent = float64(contextTokens) / float64(maxTokens)
		}
	}

	// Determine colors based on (debounced) state
	inDangerZone := tokenPercent > dangerThreshold
	isOffline := displayPresence == types.PresenceOffline
	isError := displayPresence == types.PresenceError

	// Background colors: dim grey for progress (used), explicit dark for unused
	// Using explicit color instead of transparent to prevent flicker from layout shifts
	usedBg := lipgloss.Color("236")   // dim grey (progress bar)
	unusedBg := lipgloss.Color("233") // very dark grey (nearly black, matches terminal bg)

	// Text colors: agent color for icon+name, dim grey for status
	textColor := agentColor
	statusColor := lipgloss.Color("240") // dim grey for status

	if isError {
		// Error state: red background throughout
		usedBg = lipgloss.Color("196")  // bright red
		unusedBg = lipgloss.Color("52") // dark red
		textColor = lipgloss.Color("231") // white text
		statusColor = lipgloss.Color("231") // white status too
	} else if isOffline {
		// Offline: explicit dark background, grey text
		usedBg = lipgloss.Color("233")    // very dark grey (consistent with unused)
		unusedBg = lipgloss.Color("233")  // very dark grey
		textColor = lipgloss.Color("240") // grey
		statusColor = lipgloss.Color("240") // grey
	} else if inDangerZone {
		// Danger zone (>80%): red for progress portion, dark for unused
		usedBg = lipgloss.Color("196")   // bright red
		unusedBg = lipgloss.Color("233") // very dark grey (consistent)
		textColor = lipgloss.Color("231") // white text
		statusColor = lipgloss.Color("231") // white status too
	}

	// Apply StatusDisplay color overrides
	if statusDisplay != nil {
		if statusDisplay.IconColor != nil {
			iconColor = lipgloss.Color(*statusDisplay.IconColor)
		}
		if statusDisplay.UsrColor != nil {
			textColor = lipgloss.Color(*statusDisplay.UsrColor)
		}
		if statusDisplay.MsgColor != nil {
			statusColor = lipgloss.Color(*statusDisplay.MsgColor)
		}
		if statusDisplay.UsedTokColor != nil {
			usedBg = lipgloss.Color(*statusDisplay.UsedTokColor)
		}
		if statusDisplay.UnusedTokColor != nil {
			unusedBg = lipgloss.Color(*statusDisplay.UnusedTokColor)
		}
		if statusDisplay.BgColor != nil {
			// bgcolor overrides both used and unused background
			usedBg = lipgloss.Color(*statusDisplay.BgColor)
			unusedBg = lipgloss.Color(*statusDisplay.BgColor)
		}
	}

	// Calculate fill width (how many chars get "used" background)
	var fillRatio float64
	if inDangerZone {
		// In danger zone: show progress within the remaining 20%
		// Map 80-100% to 0-100% of row
		fillRatio = (tokenPercent - dangerThreshold) / (1.0 - dangerThreshold)
		if fillRatio > 1.0 {
			fillRatio = 1.0
		}
	} else if isError || isOffline {
		// Error: full red, Offline: no fill (all black)
		if isError {
			fillRatio = 1.0
		} else {
			fillRatio = 0
		}
	} else {
		// Normal: map 0-80% to 0-100% of row
		fillRatio = tokenPercent / targetPercent
		if fillRatio > 1.0 {
			fillRatio = 1.0
		}
	}

	// Round up to next character
	fillChars := int(fillRatio*float64(rowWidth) + 0.99)
	if fillChars < 0 {
		fillChars = 0
	}
	if fillChars > rowWidth {
		fillChars = rowWidth
	}

	// Build styled row with three zones: icon, name, status
	// Icon uses iconColor (presence-based), name uses textColor (agent), status uses statusColor (dim)
	// Each zone can be in used (progress bar) or unused background
	usedIconStyle := lipgloss.NewStyle().Foreground(iconColor).Background(usedBg).Bold(true)
	usedNameStyle := lipgloss.NewStyle().Foreground(textColor).Background(usedBg).Bold(true)
	usedStatusStyle := lipgloss.NewStyle().Foreground(statusColor).Background(usedBg).Italic(true)
	unusedIconStyle := lipgloss.NewStyle().Foreground(iconColor).Background(unusedBg).Bold(true)
	unusedNameStyle := lipgloss.NewStyle().Foreground(textColor).Background(unusedBg).Bold(true)
	unusedStatusStyle := lipgloss.NewStyle().Foreground(statusColor).Background(unusedBg).Italic(true)

	runes := []rune(paddedContent)
	var rowText string

	// Helper to render a range with appropriate styles
	renderRange := func(start, end int) string {
		if start >= end || start >= len(runes) {
			return ""
		}
		if end > len(runes) {
			end = len(runes)
		}
		text := string(runes[start:end])
		// Determine which zone and background based on position
		inUsed := start < fillChars
		inIcon := start < iconEndPos
		inName := start >= iconEndPos && start < boldEndPos

		if inUsed {
			if inIcon {
				return usedIconStyle.Render(text)
			} else if inName {
				return usedNameStyle.Render(text)
			}
			return usedStatusStyle.Render(text)
		}
		// Unused background
		if inIcon {
			return unusedIconStyle.Render(text)
		} else if inName {
			return unusedNameStyle.Render(text)
		}
		return unusedStatusStyle.Render(text)
	}

	// Build segments based on boundaries (sorted)
	boundaries := []int{0, iconEndPos, boldEndPos, fillChars, rowWidth}
	// Remove duplicates and sort
	seen := make(map[int]bool)
	unique := make([]int, 0, len(boundaries))
	for _, b := range boundaries {
		if b >= 0 && b <= rowWidth && !seen[b] {
			seen[b] = true
			unique = append(unique, b)
		}
	}
	sort.Ints(unique)

	for i := 0; i < len(unique)-1; i++ {
		rowText += renderRange(unique[i], unique[i+1])
	}

	// Wrap in bubblezone for click handling
	zoneID := "agent-" + agent.AgentID
	return m.zoneManager.Mark(zoneID, rowText)
}

// dimColor returns a dimmer version of a color for the unfilled portion.
// Uses the actual color dimmed by ~50% to maintain the agent's color identity.
func dimColor(c lipgloss.Color) lipgloss.Color {
	code, ok := parseColorCode(c)
	if !ok {
		return lipgloss.Color("236") // fallback grey
	}

	// Get RGB values for the color
	r, g, b := colorCodeToRGB(code)

	// Dim by 50%
	r = r / 2
	g = g / 2
	b = b / 2

	// Convert back to 256-color code
	return lipgloss.Color(fmt.Sprintf("%d", rgbTo256(r, g, b)))
}

// rgbTo256 converts RGB values to nearest 256-color code.
func rgbTo256(r, g, b int) int {
	// Check if it's close to a grayscale value
	if abs(r-g) < 10 && abs(g-b) < 10 {
		// Use grayscale ramp (232-255)
		gray := (r + g + b) / 3
		if gray < 8 {
			return 16 // black
		}
		if gray > 238 {
			return 231 // white
		}
		return 232 + (gray-8)/10
	}

	// Use 6x6x6 color cube (16-231)
	toIndex := func(v int) int {
		if v < 48 {
			return 0
		}
		if v < 115 {
			return 1
		}
		return (v-35)/40
	}
	ri := toIndex(r)
	gi := toIndex(g)
	bi := toIndex(b)
	if ri > 5 {
		ri = 5
	}
	if gi > 5 {
		gi = 5
	}
	if bi > 5 {
		bi = 5
	}
	return 16 + ri*36 + gi*6 + bi
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// truncateStyled truncates a styled string to maxLen visible characters.
func truncateStyled(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	visibleCount := 0
	inEscape := false
	result := ""
	for _, ch := range s {
		if ch == '\x1b' {
			inEscape = true
			result += string(ch)
			continue
		}
		if inEscape {
			result += string(ch)
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEscape = false
			}
			continue
		}
		if visibleCount >= maxLen {
			break
		}
		result += string(ch)
		visibleCount++
	}
	return result
}

// colorForAgent returns the color for an agent using the same logic as chat messages.
func (m *Model) colorForAgent(agentID string) lipgloss.Color {
	return colorForAgent(agentID, m.colorMap)
}

