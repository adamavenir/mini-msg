package chat

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleSlashCommand(input string) (bool, tea.Cmd) {
	trimmed := strings.TrimSpace(input)

	// Handle click-then-command pattern: "#msg-xxx /command args" → "/command #msg-xxx args"
	if rewritten, ok := rewriteClickThenCommand(trimmed); ok {
		trimmed = rewritten
	} else if !strings.HasPrefix(trimmed, "/") {
		return false, nil
	}

	cmd, err := m.runSlashCommand(trimmed)
	if err != nil {
		m.status = err.Error()
		m.input.SetValue(input)
		m.input.CursorEnd()
		m.lastInputValue = m.input.Value()
		m.lastInputPos = m.inputCursorPos()
		return true, nil
	}

	return true, cmd
}

// rewriteClickThenCommand transforms "#id /command args" into "/command #id args".
// This allows users to click a message (which prefills #id) then type a command.
// The /command must immediately follow the #id (only whitespace allowed between).
func rewriteClickThenCommand(input string) (string, bool) {
	// Must start with # (clicked ID)
	if !strings.HasPrefix(input, "#") {
		return "", false
	}

	// Split into fields: first should be #id, second should be /command
	fields := strings.Fields(input)
	if len(fields) < 2 {
		return "", false
	}

	clickedID := fields[0]
	if !strings.HasPrefix(clickedID, "#") {
		return "", false
	}

	cmdName := fields[1]
	if !strings.HasPrefix(cmdName, "/") {
		return "", false
	}

	// Reconstruct: /command #id args...
	result := cmdName + " " + clickedID
	if len(fields) > 2 {
		result += " " + strings.Join(fields[2:], " ")
	}

	return result, true
}

func (m *Model) runSlashCommand(input string) (tea.Cmd, error) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil, nil
	}

	switch fields[0] {
	case "/quit", "/exit":
		return tea.Quit, nil
	case "/help":
		m.showHelp()
		return nil, nil
	case "/n":
		// Set nickname for selected thread
		return m.setThreadNickname(fields[1:])
	case "/f", "/fave":
		// Fave current thread (or selected thread if panel focused)
		return m.runFaveCommand(fields[1:])
	case "/unfave":
		// Unfave current thread (or explicit target)
		return m.runUnfaveCommand(fields[1:])
	case "/M", "/mute":
		// Mute current thread (or selected if panel focused)
		return m.runMuteCommand(fields[1:])
	case "/unmute":
		// Unmute current thread (or explicit target)
		return m.runUnmuteCommand(fields[1:])
	case "/follow":
		// Follow/subscribe to current thread
		return m.runFollowCommand(fields[1:])
	case "/unfollow":
		// Unfollow current thread
		return m.runUnfollowCommand(fields[1:])
	case "/archive":
		// Archive current thread (or explicit target)
		return m.runArchiveCommand(fields[1:])
	case "/restore":
		// Restore archived thread
		return m.runRestoreCommand(fields[1:])
	case "/rename":
		// Rename current thread
		return m.runRenameCommand(fields[1:])
	case "/thread", "/t":
		// Create a global (root-level) thread
		return m.runThreadCommand(input)
	case "/subthread", "/st":
		// Create a subthread of the current thread
		return m.runSubthreadCommand(input)
	case "/mv":
		// Move current thread to new parent
		return m.runMvCommand(fields[1:])
	case "/edit":
		return nil, m.runEditCommand(input)
	case "/delete", "/rm":
		return nil, m.runDeleteCommand(input)
	case "/pin":
		return nil, m.runPinCommand(fields[1:])
	case "/unpin":
		return nil, m.runUnpinCommand(fields[1:])
	case "/prune":
		return nil, m.runPruneCommand(fields[1:])
	case "/close":
		// Close all questions attached to a message
		return m.runCloseQuestionsCommand(fields[1:])
	case "/run":
		// Run mlld scripts from .fray/llm/run/
		return nil, m.runMlldScriptCommand(fields[1:])
	case "/bye":
		// Send bye for a specific agent
		return m.runByeCommand(fields[1:])
	case "/fly":
		// Spawn agent with /fly skill context
		return m.runFlyCommand(fields[1:])
	case "/hop":
		// Spawn agent with /hop skill context (auto-bye on idle)
		return m.runHopCommand(fields[1:])
	case "/land":
		// Ask active agent to run /land closeout
		return m.runLandCommand(fields[1:])
	}

	return nil, fmt.Errorf("unknown command: %s", fields[0])
}
