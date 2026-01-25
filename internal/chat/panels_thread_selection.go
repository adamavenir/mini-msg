package chat

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
	m.resize()              // Recalculate viewport height for peek statuslines
	m.refreshViewport(true) // Scroll to bottom when peeking new content
}
