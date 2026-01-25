package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

// renderSubthreadTree renders a tree preview of child threads under the anchor.
// Shows immediate children with message counts and activity indicators.
func (m *Model) renderSubthreadTree() string {
	if m.currentThread == nil {
		return ""
	}

	// Get child threads with stats
	children, err := db.GetChildThreadsWithStats(m.db, m.currentThread.GUID)
	if err != nil || len(children) == 0 {
		return ""
	}

	// Limit to first 5 children to keep it compact
	maxChildren := 5
	if len(children) > maxChildren {
		children = children[:maxChildren]
	}

	// Build tree lines
	treeStyle := lipgloss.NewStyle().Foreground(metaColor)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(metaColor).Faint(true)
	activityStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("35"))

	var lines []string
	for i, child := range children {
		// Tree branch character
		branch := "├─"
		if i == len(children)-1 {
			branch = "└─"
		}

		// Activity indicator (recent = within 1 hour)
		activityIndicator := ""
		if child.LastActivityAt != nil {
			hourAgo := time.Now().Add(-1 * time.Hour).Unix()
			if *child.LastActivityAt > hourAgo {
				activityIndicator = activityStyle.Render(" *")
			}
		}

		// Child count indicator
		childIndicator := ""
		if child.ChildCount > 0 {
			childIndicator = fmt.Sprintf(" +%d", child.ChildCount)
		}

		// Format: ├─ thread-name (5 msgs) *
		line := fmt.Sprintf("%s %s %s%s%s",
			treeStyle.Render(branch),
			m.zoneManager.Mark("subthread-"+child.GUID, nameStyle.Render(child.Name)),
			countStyle.Render(fmt.Sprintf("(%d msgs%s)", child.MessageCount, childIndicator)),
			activityIndicator,
			"",
		)
		lines = append(lines, line)
	}

	// If there are more children, show ellipsis
	totalChildren, _ := db.GetThreads(m.db, &types.ThreadQueryOptions{
		ParentThread: &m.currentThread.GUID,
	})
	if len(totalChildren) > maxChildren {
		more := len(totalChildren) - maxChildren
		lines = append(lines, treeStyle.Render(fmt.Sprintf("    ... +%d more", more)))
	}

	return strings.Join(lines, "\n")
}
