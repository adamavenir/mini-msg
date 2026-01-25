package chat

import (
	"fmt"
	"strings"
)

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
