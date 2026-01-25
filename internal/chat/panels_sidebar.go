package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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
