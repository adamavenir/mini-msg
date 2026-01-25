package chat

import (
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// isPeeking returns true if we're in peek mode (viewing content without changing posting context)
func (m *Model) isPeeking() bool {
	return m.peekSource != peekSourceNone
}

// clearPeek exits peek mode
func (m *Model) clearPeek() {
	m.peekThread = nil
	m.peekPseudo = ""
	m.peekSource = peekSourceNone
}

// commitPeek switches the posting context to match what's being peeked, then exits peek mode
func (m *Model) commitPeek() {
	// Switch to the peeked thread/pseudo
	m.currentThread = m.peekThread
	m.currentPseudo = m.peekPseudo

	// Load thread messages if switching to a thread
	if m.currentThread != nil {
		m.threadMessages, _ = db.GetThreadMessages(m.db, m.currentThread.GUID)
		m.addRecentThread(*m.currentThread)
	} else {
		m.threadMessages = nil
	}

	// Clear peek state
	m.clearPeek()

	// Refresh view and recalculate layout
	m.resize()
	m.refreshViewport(true)
}

// displayThread returns the thread whose content should be displayed
// (either the peeked thread or the current thread)
func (m *Model) displayThread() *types.Thread {
	if m.isPeeking() {
		return m.peekThread // nil means peeking main room
	}
	return m.currentThread
}

// displayPseudo returns the pseudo-thread whose content should be displayed
// (either the peeked pseudo or the current pseudo)
func (m *Model) displayPseudo() pseudoThreadKind {
	if m.isPeeking() {
		return m.peekPseudo // "" means not viewing a pseudo-thread
	}
	return m.currentPseudo
}
