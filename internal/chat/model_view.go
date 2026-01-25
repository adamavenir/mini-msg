package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) View() string {
	statusLine := lipgloss.NewStyle().Foreground(statusColor).Render(m.statusLine())

	var lines []string
	// Add pinned permission requests at top
	if pinnedPerms := m.renderPinnedPermissions(); pinnedPerms != "" {
		lines = append(lines, pinnedPerms)
	}
	// Add peek statusline at top if peeking
	if peekTop := m.renderPeekStatusline(); peekTop != "" {
		lines = append(lines, peekTop)
	}
	lines = append(lines, m.viewport.View())
	if suggestions := m.renderSuggestions(); suggestions != "" {
		lines = append(lines, suggestions)
	}
	// Add peek statusline above input if peeking, otherwise add margin
	if peekBottom := m.renderPeekStatusline(); peekBottom != "" {
		lines = append(lines, peekBottom)
	} else {
		lines = append(lines, "") // margin line when not peeking
	}
	lines = append(lines, m.renderInput(), statusLine)

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
	var output string
	if len(panels) == 0 {
		output = main
	} else {
		left := lipgloss.JoinHorizontal(lipgloss.Top, panels...)
		output = lipgloss.JoinHorizontal(lipgloss.Top, left, main)
	}
	return m.zoneManager.Scan(output)
}

// renderPeekStatusline renders a blue statusline for peek mode.
// Shows: "peeking <thread-name> · <action-hint>"
func (m *Model) renderPeekStatusline() string {
	if !m.isPeeking() {
		return ""
	}

	// Build thread name
	var name string
	if m.peekThread != nil {
		name = m.peekThread.Name
	} else if m.peekPseudo != "" {
		name = string(m.peekPseudo)
	} else {
		name = m.projectName // peeking main room
	}

	// Build action hint based on peek source
	hint := ""
	switch m.peekSource {
	case peekSourceKeyboard:
		hint = "enter to open"
	case peekSourceClick:
		hint = "click to open"
	}

	// Build content
	content := "peeking " + name
	if hint != "" {
		content += " · " + hint
	}

	// Style with blue background
	style := lipgloss.NewStyle().
		Background(peekBg).
		Foreground(lipgloss.Color("231")).
		Padding(0, 1)

	if width := m.mainWidth(); width > 0 {
		style = style.Width(width)
	}

	return style.Render(content)
}

func (m *Model) statusLine() string {
	right := ""
	if m.input.Value() == "" {
		right = "? for help"
	}
	breadcrumb := m.breadcrumb()
	left := breadcrumb
	if m.status != "" {
		left = fmt.Sprintf("%s · %s", m.status, breadcrumb)
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

func (m *Model) breadcrumb() string {
	channel := m.channelLabel()
	if m.currentPseudo != "" {
		return channel + " ❯ " + string(m.currentPseudo)
	}
	if m.currentThread == nil {
		return channel + " ❯ main"
	}
	path, err := threadPath(m.db, m.currentThread)
	if err != nil || path == "" {
		return channel + " ❯ " + m.currentThread.GUID
	}
	// Convert slash-separated path to breadcrumb with ❯
	parts := strings.Split(path, "/")
	return channel + " ❯ " + strings.Join(parts, " ❯ ")
}
