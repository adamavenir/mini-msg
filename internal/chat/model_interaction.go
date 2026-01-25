package chat

import (
	"os"
	"time"
)

const doubleClickInterval = 400 * time.Millisecond
const singleClickDebounce = 300 * time.Millisecond // Wait this long before executing single-click

// debugLog writes debug messages to a file for debugging TUI issues
func debugLog(msg string) {
	f, err := os.OpenFile("/tmp/fray-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(time.Now().Format("15:04:05.000") + " " + msg + "\n")
}

// pendingClick stores info about a click waiting for possible double-click
type pendingClick struct {
	messageID string
	zone      string // "guid", "inlineid", or "line"
	text      string // text to copy if single-click executes
	timestamp time.Time
}

// peekSourceKind indicates how a peek was triggered (for displaying action hints)
type peekSourceKind int

const (
	peekSourceNone     peekSourceKind = iota
	peekSourceKeyboard                // j/k or arrow keys
	peekSourceClick                   // mouse click
)

// clickDebounceMsg fires after singleClickDebounce to execute pending single-click
type clickDebounceMsg struct {
	messageID string
	timestamp time.Time
}
