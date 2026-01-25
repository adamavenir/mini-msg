package chat

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

// clickDebounceCmd returns a command that fires after singleClickDebounce
func (m *Model) clickDebounceCmd(messageID string, timestamp time.Time) tea.Cmd {
	return tea.Tick(singleClickDebounce, func(time.Time) tea.Msg {
		return clickDebounceMsg{messageID: messageID, timestamp: timestamp}
	})
}

// executePendingClick executes the pending single-click action
func (m *Model) executePendingClick() {
	if m.pendingClick == nil {
		return
	}
	pc := m.pendingClick
	m.pendingClick = nil

	// Commit peek since user is interacting with content
	if m.isPeeking() {
		m.commitPeek()
	}

	switch pc.zone {
	case "guid":
		// Copy the message ID to clipboard
		if err := copyToClipboard(pc.text); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Copied message ID to clipboard."
		}
	case "inlineid":
		// Copy the inline ID (without # prefix)
		if err := copyToClipboard(pc.text); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Copied ID to clipboard."
		}
	}
}

// setReplyTo sets the reply reference and shows preview (called on double-click footer ID)
func (m *Model) setReplyTo(msg types.Message) {
	debugLog(fmt.Sprintf("setReplyTo: msgID=%s from=%s", msg.ID, msg.FromAgent))
	m.replyToID = msg.ID
	m.replyToPreview = truncatePreview(msg.Body, 40)
	m.status = fmt.Sprintf("Replying to @%s", msg.FromAgent)
	m.resize() // Recalculate layout to account for reply preview line
}

// clearReplyTo clears the reply reference (called on Esc or backspace at pos 0)
func (m *Model) clearReplyTo() {
	// Debug: log caller information
	pc := make([]uintptr, 10)
	n := runtime.Callers(2, pc)
	frames := runtime.CallersFrames(pc[:n])
	var callers []string
	for {
		frame, more := frames.Next()
		callers = append(callers, fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line))
		if !more || len(callers) >= 3 {
			break
		}
	}
	debugLog(fmt.Sprintf("clearReplyTo called from: %v, replyToID was: %q", callers, m.replyToID))

	m.replyToID = ""
	m.replyToPreview = ""
	m.status = "Reply cancelled"
	m.resize() // Recalculate layout now that reply preview is gone
}
