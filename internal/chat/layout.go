package chat

const inputMaxHeight = 8
const inputPadding = 1

func (m *Model) sidebarWidth() int {
	if !m.sidebarOpen {
		return 0
	}
	return 20
}

func (m *Model) threadPanelWidth() int {
	if !m.threadPanelOpen {
		return 0
	}
	if m.cachedThreadPanelWidth > 0 {
		return m.cachedThreadPanelWidth
	}
	return 20 // fallback
}

func (m *Model) mainWidth() int {
	if m.width == 0 {
		return 0
	}
	width := m.width
	if m.threadPanelOpen {
		width -= m.threadPanelWidth()
	}
	if m.sidebarOpen {
		width -= m.sidebarWidth()
	}
	if width < 1 {
		width = 1
	}
	return width
}

func (m *Model) openThreadPanel() {
	if m.threadPanelOpen && !m.sidebarPersistent {
		// Already open and not persistent - close it
		m.threadPanelOpen = false
		m.threadPanelFocus = false
		m.resetThreadFilter()
	} else {
		// Open/focus threads panel
		m.sidebarOpen = false
		m.sidebarFocus = false
		m.resetSidebarFilter()
		m.threadPanelOpen = true
		m.threadPanelFocus = true
		if !m.sidebarPersistent {
			m.calculateThreadPanelWidth() // Calculate width when opening
		}
	}
	m.clearSuggestions()
	m.resize()
}

func (m *Model) openChannelPanel() {
	if m.sidebarOpen && !m.sidebarPersistent {
		// Already open and not persistent - close it
		m.sidebarOpen = false
		m.sidebarFocus = false
		m.resetSidebarFilter()
	} else {
		// Open/focus channels panel
		m.threadPanelOpen = false
		m.threadPanelFocus = false
		m.resetThreadFilter()
		m.sidebarOpen = true
		m.sidebarFocus = true
	}
	m.clearSuggestions()
	m.resize()
}

func (m *Model) closePanels() {
	m.threadPanelOpen = false
	m.threadPanelFocus = false
	m.sidebarOpen = false
	m.sidebarFocus = false
	m.resetThreadFilter()
	m.resetSidebarFilter()
	m.clearSuggestions()
	m.resize()
}

func (m *Model) toggleSidebarPersistence() {
	m.sidebarPersistent = !m.sidebarPersistent
}

// Legacy function for backwards compatibility
func (m *Model) cyclePanelFocus() {
	// Cycle: threads → channels → hidden → threads
	// Only one panel visible at a time
	if m.threadPanelOpen {
		// threads → channels
		m.threadPanelOpen = false
		m.threadPanelFocus = false
		m.resetThreadFilter()
		m.sidebarOpen = true
		m.sidebarFocus = true
	} else if m.sidebarOpen {
		// channels → hidden
		m.sidebarOpen = false
		m.sidebarFocus = false
		m.resetSidebarFilter()
	} else {
		// hidden → threads
		m.threadPanelOpen = true
		m.threadPanelFocus = true
		m.calculateThreadPanelWidth() // Calculate width when opening
	}
	m.clearSuggestions()
	m.resize()
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
	if m.initialScroll {
		m.refreshViewport(true)
		m.initialScroll = false
		return
	}
	m.refreshViewport(false)
}
