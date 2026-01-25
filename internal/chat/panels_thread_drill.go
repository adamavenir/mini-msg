package chat

import (
	"fmt"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

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
