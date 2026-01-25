package chat

import (
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) refreshViewport(scrollToBottom bool) {
	content := m.renderMessages()
	// Ensure content is always taller than viewport to trigger scroll behavior.
	// This works around Bubble Tea renderer bug #1232 where exact height match
	// causes first line to be cut off.
	contentHeight := lipgloss.Height(content)
	if contentHeight > 0 && contentHeight <= m.viewport.Height {
		// Add invisible padding line at top to force scrolling
		content = "\n" + content
	}
	m.viewport.SetContent(content)
	// Check pending scroll flag first
	if m.pendingScrollBottom {
		scrollToBottom = true
		m.pendingScrollBottom = false
	}
	if scrollToBottom {
		m.viewport.GotoBottom()
		m.clearNewMessageNotification() // Clear notification when scrolling to bottom
		return
	}
	if m.viewport.Height <= 0 {
		return
	}
	maxOffset := lipgloss.Height(content) - m.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.viewport.YOffset > maxOffset {
		m.viewport.SetYOffset(maxOffset)
	}
}

func (m *Model) nearTop() bool {
	return m.viewport.YOffset <= 5
}

// atBottom returns true if the viewport is scrolled to (or near) the bottom
func (m *Model) atBottom() bool {
	if m.viewport.Height <= 0 {
		return true
	}
	content := m.viewport.View()
	contentHeight := lipgloss.Height(content)
	maxOffset := contentHeight - m.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	// Consider "at bottom" if within 3 lines of the bottom
	return m.viewport.YOffset >= maxOffset-3
}

// addNewMessageAuthor adds an author to the pending new message notification list
func (m *Model) addNewMessageAuthor(author string) {
	for _, existing := range m.newMessageAuthors {
		if existing == author {
			return // already tracked
		}
	}
	m.newMessageAuthors = append(m.newMessageAuthors, author)
}

// clearNewMessageNotification clears the new message bar
func (m *Model) clearNewMessageNotification() {
	m.newMessageAuthors = nil
}

func (m *Model) loadOlderMessages() {
	if m.currentThread != nil || m.currentPseudo != "" {
		return
	}
	if !m.hasMore || m.oldestCursor == nil {
		return
	}

	options := &types.MessageQueryOptions{
		Before:          m.oldestCursor,
		Limit:           m.lastLimit,
		IncludeArchived: m.includeArchived,
	}

	prevHeight := lipgloss.Height(m.renderMessages())
	rawMessages, err := db.GetMessages(m.db, options)
	if err != nil {
		m.status = err.Error()
		return
	}
	if len(rawMessages) == 0 {
		m.hasMore = false
		return
	}

	first := rawMessages[0]
	m.oldestCursor = &types.MessageCursor{GUID: first.ID, TS: first.TS}
	if len(rawMessages) < m.lastLimit {
		m.hasMore = false
	}

	older := filterUpdates(rawMessages, m.showUpdates)
	if len(older) == 0 {
		return
	}

	m.messages = append(older, m.messages...)
	m.refreshViewport(false)
	newHeight := lipgloss.Height(m.renderMessages())
	delta := newHeight - prevHeight
	if delta > 0 {
		m.viewport.SetYOffset(m.viewport.YOffset + delta)
	}
}
