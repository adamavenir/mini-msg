package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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
		collapsedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))                                            // dim grey
		currentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24")).Bold(true)  // blue bg for current
	} else {
		// Unfocused and not peeking: muted grey tones, blue bar only on current item
		headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))                                              // grey
		itemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))                                                // dim grey
		mainStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))                                                // bright grey
		activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))                                              // grey
		selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))                                            // just brighter, no bg
		collapsedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))                                           // very dim grey
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
			if selectedPos >= m.threadScrollOffset+visibleHeight {
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
