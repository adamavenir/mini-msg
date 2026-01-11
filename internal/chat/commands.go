package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	mlld "github.com/mlld-lang/mlld/sdk/go"
)

const (
	helpLabelStyle = "\x1b[1m\x1b[97m"
	helpItemStyle  = "\x1b[94m"
	helpResetStyle = "\x1b[0m"
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

func (m *Model) setThreadNickname(args []string) (tea.Cmd, error) {
	var guid string
	var threadName string

	// Check if we have a pending nickname target from Ctrl-N
	if m.pendingNicknameGUID != "" {
		guid = m.pendingNicknameGUID
		// Find thread name for status message
		for _, t := range m.threads {
			if t.GUID == guid {
				threadName = t.Name
				break
			}
		}
		m.pendingNicknameGUID = "" // Clear after use
	} else if m.threadPanelFocus {
		// Fall back to focused thread in sidebar
		entries := m.threadEntries()
		if m.threadIndex < 0 || m.threadIndex >= len(entries) {
			return nil, fmt.Errorf("no thread selected")
		}
		entry := entries[m.threadIndex]
		if entry.Kind != threadEntryThread || entry.Thread == nil {
			return nil, fmt.Errorf("selected item is not a thread")
		}
		guid = entry.Thread.GUID
		threadName = entry.Thread.Name
	} else if m.currentThread != nil {
		// Fall back to currently viewed thread
		guid = m.currentThread.GUID
		threadName = m.currentThread.Name
	} else {
		return nil, fmt.Errorf("no thread selected (navigate to a thread or use Tab to focus sidebar)")
	}

	nickname := strings.Join(args, " ")

	if nickname == "" {
		// Clear nickname
		if err := db.SetNickname(m.db, m.username, "thread", guid, ""); err != nil {
			return nil, fmt.Errorf("failed to clear nickname: %w", err)
		}
		m.status = fmt.Sprintf("Cleared nickname for %s", threadName)
	} else {
		// Set nickname
		if err := db.SetNickname(m.db, m.username, "thread", guid, nickname); err != nil {
			return nil, fmt.Errorf("failed to set nickname: %w", err)
		}
		m.status = fmt.Sprintf("Set nickname '%s' for %s", nickname, threadName)
	}

	m.refreshThreadNicknames()
	m.input.SetValue("")
	m.input.CursorEnd()
	return nil, nil
}

func (m *Model) showHelp() {
	if m.helpMessageID != "" {
		_ = m.removeMessageByID(m.helpMessageID)
	}
	msg := newEventMessage(buildHelpText())
	m.helpMessageID = msg.ID
	m.messages = append(m.messages, msg)
	m.status = ""
	m.refreshViewport(true)
}

func buildHelpText() string {
	label := helpLabelStyle + "Shortcuts" + helpResetStyle
	lines := []string{
		label,
		formatHelpRow(
			helpItemStyle+"Ctrl-C"+helpResetStyle+" - clear text",
			helpItemStyle+"Up"+helpResetStyle+" - edit last",
			helpItemStyle+"Tab"+helpResetStyle+" - thread panel",
			25,
			22,
		),
		"",
		helpLabelStyle + "Create Threads" + helpResetStyle,
		formatHelpRow(
			helpItemStyle+"/t <name> \"anchor\""+helpResetStyle,
			helpItemStyle+"/st <name> \"anchor\""+helpResetStyle,
			"",
			24,
			22,
		),
		"",
		helpLabelStyle + "Thread Commands" + helpResetStyle + " (operate on current thread)",
		formatHelpRow(
			helpItemStyle+"/fave /unfave"+helpResetStyle,
			helpItemStyle+"/follow /unfollow"+helpResetStyle,
			helpItemStyle+"/mute /unmute"+helpResetStyle,
			18,
			22,
		),
		formatHelpRow(
			helpItemStyle+"/archive /restore"+helpResetStyle,
			helpItemStyle+"/rename <name>"+helpResetStyle,
			helpItemStyle+"/n <nick>"+helpResetStyle,
			18,
			22,
		),
		formatHelpRow(
			helpItemStyle+"/mv <dest>"+helpResetStyle+" (thread)",
			helpItemStyle+"/mv #id <dest>"+helpResetStyle+" (msg)",
			"",
			18,
			22,
		),
		"",
		helpLabelStyle + "Message Commands" + helpResetStyle,
		formatHelpRow(
			helpItemStyle+"/edit <id> <text> -m <reason>"+helpResetStyle,
			helpItemStyle+"/delete <id>"+helpResetStyle,
			"",
			35,
			22,
		),
		"",
		helpItemStyle + "Click" + helpResetStyle + " a message to reply. " +
			helpItemStyle + "Double-click" + helpResetStyle + " to copy.",
	}
	return strings.Join(lines, "\n")
}

func formatHelpRow(col1, col2, col3 string, col1Width, col2Width int) string {
	row := padHelpColumn(col1, col1Width) + padHelpColumn(col2, col2Width)
	if col3 != "" {
		row += col3
	}
	return strings.TrimRight(row, " ")
}

func padHelpColumn(value string, width int) string {
	if width <= 0 {
		return value
	}
	pad := width - ansi.StringWidth(value)
	if pad < 2 {
		pad = 2
	}
	return value + strings.Repeat(" ", pad)
}

func (m *Model) prefillEditCommand() bool {
	msg := m.lastUserMessage()
	if msg == nil {
		m.status = "No recent message to edit."
		return true
	}

	m.enterEditMode(msg.ID, msg.Body)
	return true
}

func (m *Model) enterEditMode(msgID, body string) {
	m.editingMessageID = msgID
	m.input.SetValue(body)
	m.input.CursorEnd()
	m.clearSuggestions()
	m.lastInputValue = m.input.Value()
	m.lastInputPos = m.inputCursorPos()
	m.updateInputStyle()
	m.status = fmt.Sprintf("editing #%s | Enter to save, Esc to cancel", msgID)
}

func (m *Model) exitEditMode() {
	m.editingMessageID = ""
	m.input.Reset()
	m.clearSuggestions()
	m.lastInputValue = m.input.Value()
	m.lastInputPos = m.inputCursorPos()
	m.updateInputStyle()
	m.status = ""
}

func (m *Model) submitEdit(msgID, newBody string) tea.Cmd {
	return func() tea.Msg {
		msg, err := db.GetMessage(m.db, msgID)
		if err != nil {
			return editResultMsg{err: err}
		}
		if msg == nil {
			return editResultMsg{err: fmt.Errorf("message %s not found", msgID)}
		}

		if err := db.EditMessage(m.db, msg.ID, newBody, m.username); err != nil {
			return editResultMsg{err: err}
		}

		updated, err := db.GetMessage(m.db, msg.ID)
		if err != nil {
			return editResultMsg{err: err}
		}
		if updated == nil {
			return editResultMsg{err: fmt.Errorf("message %s not found after edit", msg.ID)}
		}

		return editResultMsg{msg: updated}
	}
}

type editResultMsg struct {
	msg *types.Message
	err error
}

func (m *Model) lastUserMessage() *types.Message {
	// When in a thread, search thread messages first
	if m.currentThread != nil {
		for i := len(m.threadMessages) - 1; i >= 0; i-- {
			msg := m.threadMessages[i]
			if msg.Type != types.MessageTypeUser {
				continue
			}
			if msg.FromAgent != m.username {
				continue
			}
			if msg.ArchivedAt != nil || msg.Body == "[deleted]" {
				continue
			}
			return &msg
		}
	}
	// Search room messages
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.Type != types.MessageTypeUser {
			continue
		}
		if msg.FromAgent != m.username {
			continue
		}
		if msg.ArchivedAt != nil || msg.Body == "[deleted]" {
			continue
		}
		return &msg
	}
	return nil
}

func (m *Model) runEditCommand(input string) error {
	// Check if this is just "/edit <msg-id>" (enter edit mode)
	fields := strings.Fields(input)
	if len(fields) == 2 && fields[0] == "/edit" {
		msgID := strings.TrimPrefix(fields[1], "#")
		msg, err := m.resolveMessageInput(msgID)
		if err != nil {
			return err
		}
		m.enterEditMode(msg.ID, msg.Body)
		return nil
	}

	// Otherwise, parse as full edit command
	msgID, body, reason, err := parseEditCommand(input)
	if err != nil {
		return err
	}

	msg, err := m.resolveMessageInput(msgID)
	if err != nil {
		return err
	}

	if err := db.EditMessage(m.db, msg.ID, body, m.username); err != nil {
		return err
	}

	updated, err := db.GetMessage(m.db, msg.ID)
	if err != nil {
		return err
	}
	if updated == nil {
		return fmt.Errorf("message %s not found", msg.ID)
	}

	if err := m.appendMessageEditUpdate(*updated, reason); err != nil {
		return err
	}

	annotated, err := db.ApplyMessageEditCounts(m.projectDBPath, []types.Message{*updated})
	if err != nil {
		return err
	}
	if len(annotated) > 0 {
		*updated = annotated[0]
	}

	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	eventBody := fmt.Sprintf("edited #%s: %s", core.GetGUIDPrefix(updated.ID, prefixLength), reason)
	eventTS := time.Now().Unix()
	if updated.EditedAt != nil {
		eventTS = *updated.EditedAt
	}
	reference := updated.ID
	eventMessage, err := db.CreateMessage(m.db, types.Message{
		TS:         eventTS,
		FromAgent:  m.username,
		Body:       eventBody,
		Type:       types.MessageTypeEvent,
		References: &reference,
		Home:       "room",
	})
	if err != nil {
		return err
	}
	if err := db.AppendMessage(m.projectDBPath, eventMessage); err != nil {
		return err
	}
	m.messageCount++
	m.lastCursor = &types.MessageCursor{GUID: eventMessage.ID, TS: eventMessage.TS}

	m.applyMessageUpdate(*updated)
	if m.showUpdates {
		m.messages = append(m.messages, eventMessage)
	}
	m.refreshViewport(false)
	m.status = fmt.Sprintf("Edited #%s", updated.ID)
	return nil
}

func (m *Model) runDeleteCommand(input string) error {
	msgID, err := parseDeleteCommand(input)
	if err != nil {
		return err
	}

	msg, err := m.resolveMessageInput(msgID)
	if err != nil {
		return err
	}

	if err := db.DeleteMessage(m.db, msg.ID); err != nil {
		return err
	}

	updated, err := db.GetMessage(m.db, msg.ID)
	if err != nil {
		return err
	}
	if updated == nil {
		return fmt.Errorf("message %s not found", msg.ID)
	}

	if err := m.appendMessageUpdate(*updated); err != nil {
		return err
	}

	m.applyMessageUpdate(*updated)
	if err := m.refreshMessageCount(); err != nil {
		return err
	}
	m.refreshViewport(false)
	m.status = fmt.Sprintf("Deleted #%s", updated.ID)
	return nil
}

func (m *Model) runPruneCommand(args []string) error {
	keep, pruneAll, target, withReact, err := parsePruneArgs(args)
	if err != nil {
		return err
	}

	// Resolve target to home
	var home string
	var targetDesc string

	if target == "" {
		// Default: current thread if in thread, otherwise main room
		if m.currentThread != nil {
			home = m.currentThread.GUID
			targetDesc = m.currentThread.Name
		} else {
			home = "room"
			targetDesc = "room"
		}
	} else {
		targetLower := strings.ToLower(target)
		if targetLower == "main" || targetLower == "room" {
			home = "room"
			targetDesc = "room"
		} else {
			// Try to resolve as thread
			thread, err := m.resolveThreadRef(target)
			if err != nil {
				return err
			}
			home = thread.GUID
			targetDesc = thread.Name
		}
	}

	// Check for subthreads if pruning a thread
	if home != "room" {
		subthreads, err := db.GetThreads(m.db, &types.ThreadQueryOptions{
			ParentThread: &home,
		})
		if err != nil {
			return err
		}
		if len(subthreads) > 0 {
			return fmt.Errorf("thread has %d subthreads - cannot prune (use CLI with --include subthreads)", len(subthreads))
		}
	}

	root := projectRootFromPath(m.projectDBPath)
	if err := checkPruneGuardrails(root); err != nil {
		return err
	}

	var result pruneResult
	if withReact != "" {
		result, err = pruneMessagesWithReaction(m.projectDBPath, home, withReact)
	} else {
		result, err = pruneMessages(m.projectDBPath, keep, pruneAll, home)
	}
	if err != nil {
		return err
	}

	if err := db.RebuildDatabaseFromJSONL(m.db, m.projectDBPath); err != nil {
		return err
	}

	if err := m.reloadMessages(); err != nil {
		return err
	}

	m.input.SetValue("")
	if result.ClearedHistory {
		m.status = fmt.Sprintf("Pruned %s to last %d messages. history.jsonl cleared.", targetDesc, result.Kept)
		return nil
	}

	if withReact != "" {
		m.status = fmt.Sprintf("Pruned %d messages with %s from %s", result.Archived, withReact, targetDesc)
	} else {
		m.status = fmt.Sprintf("Pruned %s to last %d messages. Archived to history.jsonl", targetDesc, result.Kept)
	}
	return nil
}

func (m *Model) runPinCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /pin <message-id>")
	}

	msg, err := m.resolveMessageInput(args[0])
	if err != nil {
		return err
	}

	// Determine thread - use message's home if it's a thread, otherwise current thread
	var threadGUID string
	if msg.Home != "" && msg.Home != "room" {
		threadGUID = msg.Home
	} else if m.currentThread != nil {
		threadGUID = m.currentThread.GUID
	} else {
		return fmt.Errorf("message is in room; navigate to a thread first")
	}

	now := time.Now().Unix()
	if err := db.PinMessage(m.db, msg.ID, threadGUID, m.username, now); err != nil {
		return fmt.Errorf("failed to pin: %w", err)
	}

	if err := db.AppendMessagePin(m.projectDBPath, db.MessagePinJSONLRecord{
		MessageGUID: msg.ID,
		ThreadGUID:  threadGUID,
		PinnedBy:    m.username,
		PinnedAt:    now,
	}); err != nil {
		return fmt.Errorf("failed to persist pin: %w", err)
	}

	m.input.SetValue("")
	m.status = fmt.Sprintf("Pinned %s", msg.ID)
	return nil
}

func (m *Model) runUnpinCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /unpin <message-id>")
	}

	msg, err := m.resolveMessageInput(args[0])
	if err != nil {
		return err
	}

	// Determine thread - use message's home if it's a thread, otherwise current thread
	var threadGUID string
	if msg.Home != "" && msg.Home != "room" {
		threadGUID = msg.Home
	} else if m.currentThread != nil {
		threadGUID = m.currentThread.GUID
	} else {
		return fmt.Errorf("message is in room; navigate to a thread first")
	}

	if err := db.UnpinMessage(m.db, msg.ID, threadGUID); err != nil {
		return fmt.Errorf("failed to unpin: %w", err)
	}

	now := time.Now().Unix()
	if err := db.AppendMessageUnpin(m.projectDBPath, db.MessageUnpinJSONLRecord{
		MessageGUID: msg.ID,
		ThreadGUID:  threadGUID,
		UnpinnedBy:  m.username,
		UnpinnedAt:  now,
	}); err != nil {
		return fmt.Errorf("failed to persist unpin: %w", err)
	}

	m.input.SetValue("")
	m.status = fmt.Sprintf("Unpinned %s", msg.ID)
	return nil
}

func (m *Model) resolveMessageInput(raw string) (*types.Message, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(raw, "#"))
	if trimmed == "" {
		return nil, fmt.Errorf("message id is required")
	}

	msg, err := db.GetMessage(m.db, trimmed)
	if err != nil {
		return nil, err
	}
	if msg != nil {
		return msg, nil
	}

	if !strings.HasPrefix(strings.ToLower(trimmed), "msg-") {
		msg, err = db.GetMessage(m.db, "msg-"+trimmed)
		if err != nil {
			return nil, err
		}
		if msg != nil {
			return msg, nil
		}
	}

	msg, err = db.GetMessageByPrefix(m.db, trimmed)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("message not found: %s", trimmed)
	}
	return msg, nil
}

func (m *Model) appendMessageUpdate(msg types.Message) error {
	body := msg.Body
	update := db.MessageUpdateJSONLRecord{ID: msg.ID, Body: &body}
	if msg.EditedAt != nil {
		update.EditedAt = msg.EditedAt
	}
	if msg.ArchivedAt != nil {
		update.ArchivedAt = msg.ArchivedAt
	}
	return db.AppendMessageUpdate(m.projectDBPath, update)
}

func (m *Model) appendMessageEditUpdate(msg types.Message, reason string) error {
	body := msg.Body
	update := db.MessageUpdateJSONLRecord{ID: msg.ID, Body: &body, Reason: &reason}
	if msg.EditedAt != nil {
		update.EditedAt = msg.EditedAt
	}
	return db.AppendMessageUpdate(m.projectDBPath, update)
}

func (m *Model) applyMessageUpdate(msg types.Message) {
	for i := range m.messages {
		if m.messages[i].ID != msg.ID {
			continue
		}
		if !m.includeArchived && msg.ArchivedAt != nil {
			m.messages = append(m.messages[:i], m.messages[i+1:]...)
			break
		}
		m.messages[i] = msg
		break
	}
	for i := range m.threadMessages {
		if m.threadMessages[i].ID != msg.ID {
			continue
		}
		if !m.includeArchived && msg.ArchivedAt != nil {
			m.threadMessages = append(m.threadMessages[:i], m.threadMessages[i+1:]...)
			return
		}
		m.threadMessages[i] = msg
		return
	}
}

func (m *Model) refreshMessageCount() error {
	count, err := countMessages(m.db, m.includeArchived)
	if err != nil {
		return err
	}
	m.messageCount = count
	return nil
}

func (m *Model) reloadMessages() error {
	rawMessages, err := db.GetMessages(m.db, &types.MessageQueryOptions{
		Limit:           m.lastLimit,
		IncludeArchived: m.includeArchived,
	})
	if err != nil {
		return err
	}
	rawMessages, err = db.ApplyMessageEditCounts(m.projectDBPath, rawMessages)
	if err != nil {
		return err
	}
	messages := filterUpdates(rawMessages, m.showUpdates)

	var lastCursor *types.MessageCursor
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		lastCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}
	}

	var oldestCursor *types.MessageCursor
	if len(rawMessages) > 0 {
		first := rawMessages[0]
		oldestCursor = &types.MessageCursor{GUID: first.ID, TS: first.TS}
	}

	count, err := countMessages(m.db, m.includeArchived)
	if err != nil {
		return err
	}

	colorMap, err := buildColorMap(m.db, 50, m.includeArchived)
	if err != nil {
		return err
	}

	m.messages = messages
	m.lastCursor = lastCursor
	m.oldestCursor = oldestCursor
	m.messageCount = count
	m.hasMore = len(rawMessages) < count
	m.colorMap = colorMap
	m.refreshViewport(true)
	return nil
}

func parseEditCommand(input string) (string, string, string, error) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/edit") {
		return "", "", "", fmt.Errorf("usage: /edit <msgid> <text> -m <reason>")
	}

	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "/edit"))
	if rest == "" {
		return "", "", "", fmt.Errorf("usage: /edit <msgid> <text> -m <reason>")
	}

	fields := strings.Fields(rest)
	if len(fields) < 4 {
		return "", "", "", fmt.Errorf("usage: /edit <msgid> <text> -m <reason>")
	}

	id := strings.TrimSpace(fields[0])
	bodyParts := []string{}
	reasonParts := []string{}
	seenFlag := false
	for i := 1; i < len(fields); i++ {
		if fields[i] == "-m" || fields[i] == "--message" {
			seenFlag = true
			reasonParts = fields[i+1:]
			break
		}
		bodyParts = append(bodyParts, fields[i])
	}
	if !seenFlag || len(bodyParts) == 0 || len(reasonParts) == 0 {
		return "", "", "", fmt.Errorf("usage: /edit <msgid> <text> -m <reason>")
	}
	body := strings.TrimSpace(strings.Join(bodyParts, " "))
	reason := strings.TrimSpace(strings.Join(reasonParts, " "))
	if body == "" || reason == "" || id == "" {
		return "", "", "", fmt.Errorf("usage: /edit <msgid> <text> -m <reason>")
	}

	id = strings.TrimPrefix(id, "#")
	return id, body, reason, nil
}

func parseDeleteCommand(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/delete") && !strings.HasPrefix(trimmed, "/rm") {
		return "", fmt.Errorf("usage: /delete <msgid>")
	}

	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", fmt.Errorf("usage: /delete <msgid>")
	}

	id := strings.TrimPrefix(fields[1], "#")
	if id == "" {
		return "", fmt.Errorf("usage: /delete <msgid>")
	}

	return id, nil
}

func parsePruneArgs(args []string) (int, bool, string, string, error) {
	keep := 20
	pruneAll := false
	target := ""
	withReact := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--all":
			pruneAll = true
		case arg == "--keep":
			if i+1 >= len(args) {
				return 0, false, "", "", fmt.Errorf("usage: /prune [target] [--keep N] [--all] [--with-react emoji]")
			}
			i++
			value, err := parseNonNegativeInt(args[i])
			if err != nil {
				return 0, false, "", "", err
			}
			keep = value
		case strings.HasPrefix(arg, "--keep="):
			value, err := parseNonNegativeInt(strings.TrimPrefix(arg, "--keep="))
			if err != nil {
				return 0, false, "", "", err
			}
			keep = value
		case arg == "--with-react":
			if i+1 >= len(args) {
				return 0, false, "", "", fmt.Errorf("usage: /prune [target] [--with-react emoji]")
			}
			i++
			withReact = args[i]
		case strings.HasPrefix(arg, "--with-react="):
			withReact = strings.TrimPrefix(arg, "--with-react=")
		case strings.HasPrefix(arg, "--"):
			return 0, false, "", "", fmt.Errorf("unknown flag: %s", arg)
		default:
			// First non-flag arg is target, or could be a number for keep
			if target == "" {
				// Check if it's a number (legacy keep syntax)
				if value, err := parseNonNegativeInt(arg); err == nil {
					keep = value
				} else {
					target = arg
				}
			} else {
				return 0, false, "", "", fmt.Errorf("usage: /prune [target] [--keep N] [--all] [--with-react emoji]")
			}
		}
	}

	return keep, pruneAll, target, withReact, nil
}

func parseNonNegativeInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid value: %s", value)
	}
	return parsed, nil
}

// getTargetThread returns the thread to operate on: either from explicit arg or current thread.
func (m *Model) getTargetThread(args []string) (*types.Thread, error) {
	if len(args) > 0 {
		thread, err := m.resolveThreadRef(args[0])
		if err != nil {
			return nil, err
		}
		return thread, nil
	}
	if m.currentThread == nil {
		return nil, fmt.Errorf("no thread selected (use main to navigate, or specify thread)")
	}
	return m.currentThread, nil
}

// resolveThreadRef finds a thread by GUID, prefix, or name.
func (m *Model) resolveThreadRef(ref string) (*types.Thread, error) {
	value := strings.TrimSpace(strings.TrimPrefix(ref, "#"))
	if value == "" {
		return nil, fmt.Errorf("thread reference is required")
	}

	// Try exact GUID match
	thread, err := db.GetThread(m.db, value)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	// Try prefix match
	thread, err = db.GetThreadByPrefix(m.db, value)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	// Try name match (root-level first for backward compat)
	thread, err = db.GetThreadByName(m.db, value, nil)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	// Try name match (any level)
	thread, err = db.GetThreadByNameAny(m.db, value)
	if err != nil {
		return nil, err
	}
	if thread != nil {
		return thread, nil
	}

	return nil, fmt.Errorf("thread not found: %s", ref)
}

func (m *Model) runFaveCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if m.favedThreads[thread.GUID] {
		m.status = fmt.Sprintf("%s is already faved", thread.Name)
		return nil, nil
	}

	favedAt, err := db.AddFave(m.db, m.username, "thread", thread.GUID)
	if err != nil {
		return nil, fmt.Errorf("failed to fave: %w", err)
	}
	if err := db.AppendAgentFave(m.projectDBPath, m.username, "thread", thread.GUID, favedAt); err != nil {
		return nil, fmt.Errorf("failed to persist fave: %w", err)
	}

	// Also subscribe to the thread (faving implies following)
	now := time.Now().Unix()
	if !m.subscribedThreads[thread.GUID] {
		if err := db.SubscribeThread(m.db, thread.GUID, m.username, now); err == nil {
			_ = db.AppendThreadSubscribe(m.projectDBPath, db.ThreadSubscribeJSONLRecord{
				ThreadGUID:   thread.GUID,
				AgentID:      m.username,
				SubscribedAt: now,
			})
			m.refreshSubscribedThreads()
		}
	}

	m.refreshFavedThreads()
	m.status = fmt.Sprintf("Faved %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runUnfaveCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if !m.favedThreads[thread.GUID] {
		m.status = fmt.Sprintf("%s is not faved", thread.Name)
		return nil, nil
	}

	if err := db.RemoveFave(m.db, m.username, "thread", thread.GUID); err != nil {
		return nil, fmt.Errorf("failed to unfave: %w", err)
	}
	if err := db.AppendAgentUnfave(m.projectDBPath, m.username, "thread", thread.GUID, time.Now().Unix()); err != nil {
		return nil, fmt.Errorf("failed to persist unfave: %w", err)
	}

	m.refreshFavedThreads()
	m.status = fmt.Sprintf("Unfaved %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runMuteCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if m.mutedThreads[thread.GUID] {
		m.status = fmt.Sprintf("%s is already muted", thread.Name)
		return nil, nil
	}

	if err := db.MuteThread(m.db, thread.GUID, m.username, 0, nil); err != nil {
		return nil, fmt.Errorf("failed to mute: %w", err)
	}

	m.refreshMutedThreads()
	m.status = fmt.Sprintf("Muted %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runUnmuteCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if !m.mutedThreads[thread.GUID] {
		m.status = fmt.Sprintf("%s is not muted", thread.Name)
		return nil, nil
	}

	if err := db.UnmuteThread(m.db, thread.GUID, m.username); err != nil {
		return nil, fmt.Errorf("failed to unmute: %w", err)
	}

	m.refreshMutedThreads()
	m.status = fmt.Sprintf("Unmuted %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runFollowCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if m.subscribedThreads[thread.GUID] {
		m.status = fmt.Sprintf("Already following %s", thread.Name)
		return nil, nil
	}

	now := time.Now().Unix()
	if err := db.SubscribeThread(m.db, thread.GUID, m.username, now); err != nil {
		return nil, fmt.Errorf("failed to follow: %w", err)
	}
	if err := db.AppendThreadSubscribe(m.projectDBPath, db.ThreadSubscribeJSONLRecord{
		ThreadGUID:   thread.GUID,
		AgentID:      m.username,
		SubscribedAt: now,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist follow: %w", err)
	}

	m.refreshSubscribedThreads()
	m.status = fmt.Sprintf("Following %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runUnfollowCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if !m.subscribedThreads[thread.GUID] {
		m.status = fmt.Sprintf("Not following %s", thread.Name)
		return nil, nil
	}

	if err := db.UnsubscribeThread(m.db, thread.GUID, m.username); err != nil {
		return nil, fmt.Errorf("failed to unfollow: %w", err)
	}
	if err := db.AppendThreadUnsubscribe(m.projectDBPath, db.ThreadUnsubscribeJSONLRecord{
		ThreadGUID:     thread.GUID,
		AgentID:        m.username,
		UnsubscribedAt: time.Now().Unix(),
	}); err != nil {
		return nil, fmt.Errorf("failed to persist unfollow: %w", err)
	}

	m.refreshSubscribedThreads()
	m.status = fmt.Sprintf("Unfollowed %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runArchiveCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if thread.Status == types.ThreadStatusArchived {
		m.status = fmt.Sprintf("%s is already archived", thread.Name)
		return nil, nil
	}

	statusValue := string(types.ThreadStatusArchived)
	_, err = db.UpdateThread(m.db, thread.GUID, db.ThreadUpdates{
		Status: types.OptionalString{Set: true, Value: &statusValue},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to archive: %w", err)
	}
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID:   thread.GUID,
		Status: &statusValue,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist archive: %w", err)
	}

	// If archived current thread, navigate away
	if m.currentThread != nil && m.currentThread.GUID == thread.GUID {
		m.currentThread = nil
		m.threadMessages = nil
		m.refreshViewport(true)
	}

	m.status = fmt.Sprintf("Archived %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runRestoreCommand(args []string) (tea.Cmd, error) {
	thread, err := m.getTargetThread(args)
	if err != nil {
		return nil, err
	}

	if thread.Status != types.ThreadStatusArchived {
		m.status = fmt.Sprintf("%s is not archived", thread.Name)
		return nil, nil
	}

	statusValue := string(types.ThreadStatusOpen)
	_, err = db.UpdateThread(m.db, thread.GUID, db.ThreadUpdates{
		Status: types.OptionalString{Set: true, Value: &statusValue},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to restore: %w", err)
	}
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID:   thread.GUID,
		Status: &statusValue,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist restore: %w", err)
	}

	m.status = fmt.Sprintf("Restored %s", thread.Name)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runRenameCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /rename <new-name>")
	}

	if m.currentThread == nil {
		return nil, fmt.Errorf("no thread selected (navigate to a thread first)")
	}

	newName := strings.Join(args, " ")

	// Check for duplicate name
	var parentGUID *string
	if m.currentThread.ParentThread != nil {
		parentGUID = m.currentThread.ParentThread
	}
	existing, err := db.GetThreadByName(m.db, newName, parentGUID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.GUID != m.currentThread.GUID {
		return nil, fmt.Errorf("thread already exists: %s", newName)
	}

	_, err = db.UpdateThread(m.db, m.currentThread.GUID, db.ThreadUpdates{
		Name: types.OptionalString{Set: true, Value: &newName},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to rename: %w", err)
	}
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID: m.currentThread.GUID,
		Name: &newName,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist rename: %w", err)
	}

	oldName := m.currentThread.Name
	m.currentThread.Name = newName
	m.status = fmt.Sprintf("Renamed %s to %s", oldName, newName)
	m.input.SetValue("")
	return nil, nil
}

func (m *Model) runMvCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /mv [#msg-id] <destination>")
	}

	// Check if first arg is a message reference
	firstArg := args[0]
	if strings.HasPrefix(firstArg, "#") || strings.HasPrefix(strings.ToLower(firstArg), "msg-") {
		// Moving a message
		return m.runMvMessageCommand(args)
	}

	// Moving current thread
	return m.runMvThreadCommand(args)
}

// isRoomDestination checks if the destination refers to the main room.
func (m *Model) isRoomDestination(dest string) bool {
	destLower := strings.ToLower(dest)
	return destLower == "room" || destLower == "main" || destLower == m.projectName
}

// runMvMessageCommand moves a message to a thread or room.
func (m *Model) runMvMessageCommand(args []string) (tea.Cmd, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("usage: /mv #msg-id <destination>")
	}

	msgRef := strings.TrimPrefix(args[0], "#")
	destRef := args[1]

	// Resolve message
	msg, err := m.resolveMessageInput(msgRef)
	if err != nil {
		return nil, fmt.Errorf("message not found: %s", msgRef)
	}

	// Resolve destination
	var newHome string
	if m.isRoomDestination(destRef) {
		newHome = "room"
	} else {
		destThread, err := m.resolveThreadRef(destRef)
		if err != nil {
			return nil, fmt.Errorf("thread not found: %s", destRef)
		}
		newHome = destThread.GUID
	}

	// Check if already there
	if msg.Home == newHome {
		return nil, fmt.Errorf("message is already in %s", destRef)
	}

	now := time.Now().Unix()
	oldHome := msg.Home

	// Move the message
	if err := db.MoveMessage(m.db, msg.ID, newHome); err != nil {
		return nil, fmt.Errorf("failed to move: %w", err)
	}

	if err := db.AppendMessageMove(m.projectDBPath, db.MessageMoveJSONLRecord{
		MessageGUID: msg.ID,
		OldHome:     oldHome,
		NewHome:     newHome,
		MovedBy:     m.username,
		MovedAt:     now,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist move: %w", err)
	}

	// Update thread activity if moving to a thread
	if newHome != "room" {
		if err := db.UpdateThreadActivity(m.db, newHome, now); err != nil {
			return nil, err
		}
		if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
			GUID:           newHome,
			LastActivityAt: &now,
		}); err != nil {
			return nil, err
		}
	}

	// Update display
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	msgPrefix := core.GetGUIDPrefix(msg.ID, prefixLength)
	destName := destRef
	if newHome == "room" {
		destName = "main"
	}
	m.status = fmt.Sprintf("Moved #%s to %s", msgPrefix, destName)
	m.input.SetValue("")
	return nil, nil
}

// runMvThreadCommand moves the current thread to a new parent.
func (m *Model) runMvThreadCommand(args []string) (tea.Cmd, error) {
	if m.currentThread == nil {
		return nil, fmt.Errorf("no thread selected (navigate to a thread first, or specify a message: /mv #msg-id destination)")
	}

	destRef := args[0]
	var anchorText string
	if len(args) > 1 {
		anchorText = strings.Join(args[1:], " ")
	}

	// Resolve destination (new parent)
	var newParentGUID *string
	destLower := strings.ToLower(destRef)
	if destLower == "root" || destLower == "/" {
		// Move to root (no parent)
		newParentGUID = nil
	} else {
		destThread, err := m.resolveThreadRef(destRef)
		if err != nil {
			return nil, fmt.Errorf("thread not found: %s", destRef)
		}
		newParentGUID = &destThread.GUID

		// Check for cycles: can't move thread under one of its descendants
		isDescendant, err := m.isAncestorOf(destThread.GUID, m.currentThread.GUID)
		if err != nil {
			return nil, err
		}
		if isDescendant {
			return nil, fmt.Errorf("cannot move %s under %s: would create a cycle", m.currentThread.Name, destThread.Name)
		}
	}

	// Check if already at target parent
	currentParent := ""
	if m.currentThread.ParentThread != nil {
		currentParent = *m.currentThread.ParentThread
	}
	targetParent := ""
	if newParentGUID != nil {
		targetParent = *newParentGUID
	}
	if currentParent == targetParent {
		return nil, fmt.Errorf("%s is already under that parent", m.currentThread.Name)
	}

	now := time.Now().Unix()

	// Update thread parent
	_, err := db.UpdateThread(m.db, m.currentThread.GUID, db.ThreadUpdates{
		ParentThread: types.OptionalString{Set: true, Value: newParentGUID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to move: %w", err)
	}

	// Persist to JSONL
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID:         m.currentThread.GUID,
		ParentThread: newParentGUID,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist move: %w", err)
	}

	// If anchor text provided, create anchor message
	if anchorText != "" {
		agentBases, err := db.GetAgentBases(m.db)
		if err != nil {
			return nil, err
		}
		mentions := core.ExtractMentions(anchorText, agentBases)
		mentions = core.ExpandAllMention(mentions, agentBases)

		newMsg := types.Message{
			TS:        now,
			Home:      m.currentThread.GUID,
			FromAgent: m.username,
			Body:      anchorText,
			Mentions:  mentions,
			Type:      types.MessageTypeUser,
		}

		created, err := db.CreateMessage(m.db, newMsg)
		if err != nil {
			return nil, err
		}

		if err := db.AppendMessage(m.projectDBPath, created); err != nil {
			return nil, err
		}

		// Set as anchor
		_, err = db.UpdateThread(m.db, m.currentThread.GUID, db.ThreadUpdates{
			AnchorMessageGUID: types.OptionalString{Set: true, Value: &created.ID},
		})
		if err != nil {
			return nil, err
		}

		if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
			GUID:              m.currentThread.GUID,
			AnchorMessageGUID: &created.ID,
		}); err != nil {
			return nil, err
		}
	}

	// Update thread activity
	if err := db.UpdateThreadActivity(m.db, m.currentThread.GUID, now); err != nil {
		return nil, err
	}
	if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
		GUID:           m.currentThread.GUID,
		LastActivityAt: &now,
	}); err != nil {
		return nil, err
	}

	// Update local state
	m.currentThread.ParentThread = newParentGUID

	destName := "root"
	if newParentGUID != nil {
		destThread, _ := db.GetThread(m.db, *newParentGUID)
		if destThread != nil {
			destName = destThread.Name
		}
	}

	if anchorText != "" {
		m.status = fmt.Sprintf("Moved %s under %s (with anchor)", m.currentThread.Name, destName)
	} else {
		m.status = fmt.Sprintf("Moved %s under %s", m.currentThread.Name, destName)
	}
	m.input.SetValue("")
	return nil, nil
}

// isAncestorOf checks if potentialAncestor is an ancestor of threadGUID.
func (m *Model) isAncestorOf(threadGUID, potentialAncestorGUID string) (bool, error) {
	if threadGUID == potentialAncestorGUID {
		return true, nil
	}
	current := threadGUID
	seen := map[string]struct{}{}
	for {
		if _, ok := seen[current]; ok {
			return false, fmt.Errorf("thread parent loop detected")
		}
		seen[current] = struct{}{}
		thread, err := db.GetThread(m.db, current)
		if err != nil {
			return false, err
		}
		if thread == nil || thread.ParentThread == nil || *thread.ParentThread == "" {
			return false, nil
		}
		if *thread.ParentThread == potentialAncestorGUID {
			return true, nil
		}
		current = *thread.ParentThread
	}
}

// runThreadCommand creates a new root-level thread.
// Syntax: /thread name "anchor" or /t name "anchor"
func (m *Model) runThreadCommand(input string) (tea.Cmd, error) {
	name, anchor, err := parseThreadArgs(input)
	if err != nil {
		return nil, err
	}

	return m.createThread(name, nil, anchor)
}

// runSubthreadCommand creates a subthread under the current thread.
// Syntax: /subthread name "anchor" or /st name "anchor"
func (m *Model) runSubthreadCommand(input string) (tea.Cmd, error) {
	if m.currentThread == nil {
		return nil, fmt.Errorf("navigate to a thread first to create a subthread")
	}

	name, anchor, err := parseThreadArgs(input)
	if err != nil {
		return nil, err
	}

	return m.createThread(name, &m.currentThread.GUID, anchor)
}

// createThread creates a thread with optional parent and anchor.
func (m *Model) createThread(name string, parentGUID *string, anchorText string) (tea.Cmd, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("thread name is required")
	}
	if strings.Contains(name, "/") {
		return nil, fmt.Errorf("thread name cannot contain '/'")
	}

	// Check if thread already exists at this level
	existing, err := db.GetThreadByName(m.db, name, parentGUID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("thread already exists: %s", name)
	}

	// Check for meta/ path collision (e.g., creating "opus/notes" when "meta/opus/notes" exists)
	if err := m.checkMetaPathCollision(parentGUID, name); err != nil {
		return nil, err
	}

	// Create the thread
	thread, err := db.CreateThread(m.db, types.Thread{
		Name:         name,
		ParentThread: parentGUID,
		Status:       types.ThreadStatusOpen,
	})
	if err != nil {
		return nil, err
	}

	// Persist to JSONL with current user as subscriber
	subscribers := []string{m.username}
	if err := db.AppendThread(m.projectDBPath, thread, subscribers); err != nil {
		return nil, err
	}

	now := time.Now().Unix()

	// Subscribe creator to the thread
	if err := db.SubscribeThread(m.db, thread.GUID, m.username, now); err != nil {
		return nil, err
	}

	// Create anchor message if provided
	if anchorText != "" {
		agentBases, err := db.GetAgentBases(m.db)
		if err != nil {
			return nil, err
		}
		mentions := core.ExtractMentions(anchorText, agentBases)
		mentions = core.ExpandAllMention(mentions, agentBases)

		newMsg := types.Message{
			TS:        now,
			Home:      thread.GUID,
			FromAgent: m.username,
			Body:      anchorText,
			Mentions:  mentions,
			Type:      types.MessageTypeUser,
		}

		created, err := db.CreateMessage(m.db, newMsg)
		if err != nil {
			return nil, err
		}

		if err := db.AppendMessage(m.projectDBPath, created); err != nil {
			return nil, err
		}

		// Set as anchor
		_, err = db.UpdateThread(m.db, thread.GUID, db.ThreadUpdates{
			AnchorMessageGUID: types.OptionalString{Set: true, Value: &created.ID},
			LastActivityAt:    types.OptionalInt64{Set: true, Value: &now},
		})
		if err != nil {
			return nil, err
		}

		if err := db.AppendThreadUpdate(m.projectDBPath, db.ThreadUpdateJSONLRecord{
			GUID:              thread.GUID,
			AnchorMessageGUID: &created.ID,
			LastActivityAt:    &now,
		}); err != nil {
			return nil, err
		}
	}

	// Navigate to the new thread
	m.currentThread = &thread
	m.currentPseudo = ""
	m.threadMessages = nil
	m.refreshSubscribedThreads()

	// Build path for status message
	path := name
	if parentGUID != nil {
		parentThread, _ := db.GetThread(m.db, *parentGUID)
		if parentThread != nil {
			parentPath, _ := threadPath(m.db, parentThread)
			if parentPath != "" {
				path = parentPath + "/" + name
			}
		}
	}

	if anchorText != "" {
		m.status = fmt.Sprintf("Created %s with anchor", path)
	} else {
		m.status = fmt.Sprintf("Created %s", path)
	}
	m.input.SetValue("")
	m.refreshViewport(true)
	return nil, nil
}

// parseThreadArgs extracts name and anchor from /thread or /subthread input.
// Supports: /thread name "anchor" or /thread name
func parseThreadArgs(input string) (string, string, error) {
	// Remove command prefix
	trimmed := strings.TrimSpace(input)
	rest := ""
	for _, prefix := range []string{"/subthread ", "/st ", "/thread ", "/t "} {
		if strings.HasPrefix(trimmed, prefix) {
			rest = strings.TrimSpace(trimmed[len(prefix):])
			break
		}
	}
	if rest == "" {
		return "", "", fmt.Errorf("usage: /thread name [\"anchor\"] or /t name [\"anchor\"]")
	}

	// Check for quoted anchor
	quoteIdx := strings.Index(rest, "\"")
	if quoteIdx == -1 {
		// No anchor, just name
		return strings.TrimSpace(rest), "", nil
	}

	name := strings.TrimSpace(rest[:quoteIdx])
	if name == "" {
		return "", "", fmt.Errorf("thread name is required")
	}

	// Extract quoted anchor
	anchorPart := rest[quoteIdx:]
	if len(anchorPart) < 2 || !strings.HasPrefix(anchorPart, "\"") {
		return "", "", fmt.Errorf("anchor must be quoted: /thread name \"anchor text\"")
	}

	// Find closing quote
	closingIdx := strings.LastIndex(anchorPart, "\"")
	if closingIdx <= 0 {
		return "", "", fmt.Errorf("missing closing quote for anchor")
	}

	anchor := anchorPart[1:closingIdx]
	return name, anchor, nil
}

// checkMetaPathCollision checks if creating a thread would collide with a meta/ equivalent.
// For example, creating "opus/notes" when "meta/opus/notes" exists is likely an error.
func (m *Model) checkMetaPathCollision(parentGUID *string, name string) error {
	// Build the full path that would be created
	var fullPath string
	if parentGUID == nil {
		fullPath = name
	} else {
		parentThread, err := db.GetThread(m.db, *parentGUID)
		if err != nil {
			return err
		}
		if parentThread == nil {
			return nil
		}
		parentPath, err := threadPath(m.db, parentThread)
		if err != nil {
			return err
		}
		fullPath = parentPath + "/" + name
	}

	// Skip if path already starts with meta
	if strings.HasPrefix(fullPath, "meta/") || fullPath == "meta" {
		return nil
	}

	// Check if meta/<path> exists
	metaPath := "meta/" + fullPath
	parts := strings.Split(metaPath, "/")
	var parent *types.Thread
	for _, part := range parts {
		var parentGUID *string
		if parent != nil {
			parentGUID = &parent.GUID
		}
		thread, err := db.GetThreadByName(m.db, part, parentGUID)
		if err != nil {
			return nil // Error checking = no collision
		}
		if thread == nil {
			return nil // Path doesn't exist = no collision
		}
		parent = thread
	}

	// If we got here, the full meta path exists
	return fmt.Errorf("thread exists at %s - use that path instead", metaPath)
}

// runCloseQuestionsCommand closes all questions attached to a message.
// Syntax: /close #msg-xyz or /close (uses last selected in open-qs)
func (m *Model) runCloseQuestionsCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("/close requires a message ID (#msg-xyz)")
	}

	// Parse message ID from args
	ref := args[0]
	if strings.HasPrefix(ref, "#") {
		ref = ref[1:]
	}

	// Resolve message GUID from prefix (use ResolveReplyReference logic)
	resolution, err := ResolveReplyReference(m.db, "#"+ref)
	if err != nil {
		return nil, fmt.Errorf("could not resolve message: %v", err)
	}
	if resolution.Kind == ReplyNone {
		return nil, fmt.Errorf("no message found matching: #%s", ref)
	}
	if resolution.Kind == ReplyAmbiguous {
		return nil, fmt.Errorf("ambiguous message reference: #%s", ref)
	}

	messageID := resolution.ReplyTo

	// Find all questions attached to this message
	questions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		AskedIn: &messageID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find questions: %v", err)
	}

	if len(questions) == 0 {
		return nil, fmt.Errorf("no questions found for message #%s", ref)
	}

	// Close all questions
	closed := 0
	for _, q := range questions {
		if q.Status == types.QuestionStatusAnswered {
			continue // Already closed
		}
		status := string(types.QuestionStatusAnswered)
		_, err := db.UpdateQuestion(m.db, q.GUID, db.QuestionUpdates{
			Status: types.OptionalString{Set: true, Value: &status},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to close question: %v", err)
		}

		// Write update to JSONL
		_ = db.AppendQuestionUpdate(m.projectDBPath, db.QuestionUpdateJSONLRecord{
			Type:   "question_update",
			GUID:   q.GUID,
			Status: &status,
		})
		closed++
	}

	if closed == 0 {
		m.status = fmt.Sprintf("All %d questions already closed", len(questions))
	} else {
		m.status = fmt.Sprintf("Closed %d question(s) for #%s", closed, ref)
	}

	// Refresh question counts
	m.refreshQuestionCounts()

	return nil, nil
}

// runMlldScriptCommand runs mlld scripts from .fray/llm/run/ or llm/run/.
// With no args, lists available scripts. With a script name, runs it.
// Scripts in .fray/llm/run/ run from .fray/, scripts in llm/run/ run from project root.
func (m *Model) runMlldScriptCommand(args []string) error {
	frayRunDir := filepath.Join(m.projectRoot, ".fray", "llm", "run")
	projRunDir := filepath.Join(m.projectRoot, "llm", "run")

	// Collect scripts from both locations
	var allScripts []string
	seen := make(map[string]bool)

	if scripts, err := listMlldScripts(frayRunDir); err == nil {
		for _, s := range scripts {
			if !seen[s] {
				seen[s] = true
				allScripts = append(allScripts, s)
			}
		}
	}
	if scripts, err := listMlldScripts(projRunDir); err == nil {
		for _, s := range scripts {
			if !seen[s] {
				seen[s] = true
				allScripts = append(allScripts, s)
			}
		}
	}

	if len(args) == 0 {
		// List scripts
		if len(allScripts) == 0 {
			m.status = "No scripts found (create .fray/llm/run/*.mld or llm/run/*.mld)"
			return nil
		}
		lines := []string{"Available scripts:"}
		for _, name := range allScripts {
			lines = append(lines, "  /run "+name)
		}
		msg := newEventMessage(strings.Join(lines, "\n"))
		m.messages = append(m.messages, msg)
		m.refreshViewport(true)
		return nil
	}

	// Find and run the specified script
	scriptName := args[0]

	// Check fray location first, then project location
	var scriptPath string
	var workingDir string

	frayPath := filepath.Join(frayRunDir, scriptName+".mld")
	projPath := filepath.Join(projRunDir, scriptName+".mld")

	if _, err := os.Stat(frayPath); err == nil {
		scriptPath = frayPath
		workingDir = filepath.Join(m.projectRoot, ".fray")
	} else if _, err := os.Stat(projPath); err == nil {
		scriptPath = projPath
		workingDir = m.projectRoot
	} else {
		return fmt.Errorf("script not found: %s", scriptName)
	}

	// Execute with mlld using the SDK
	client := mlld.New()
	client.Timeout = 5 * time.Minute
	client.WorkingDir = workingDir

	m.status = fmt.Sprintf("Running %s...", scriptName)

	result, err := client.Execute(scriptPath, nil, nil)
	if err != nil {
		return fmt.Errorf("script error: %v", err)
	}

	// Display output
	output := strings.TrimSpace(result.Output)
	if output != "" {
		msg := newEventMessage(fmt.Sprintf("[%s]\n%s", scriptName, output))
		m.messages = append(m.messages, msg)
	}

	m.status = fmt.Sprintf("Ran %s", scriptName)
	m.refreshViewport(true)
	m.input.SetValue("")
	return nil
}

// listMlldScripts returns names of .mld files in the given directory.
func listMlldScripts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var scripts []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".mld") {
			scripts = append(scripts, strings.TrimSuffix(name, ".mld"))
		}
	}
	return scripts, nil
}

// runByeCommand sends bye for a specific agent.
// Syntax: /bye @agent [message]
func (m *Model) runByeCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /bye @agent [message]")
	}

	// Parse agent ID (strip @ prefix if present)
	agentRef := args[0]
	agentID := strings.TrimPrefix(agentRef, "@")
	if agentID == "" {
		return nil, fmt.Errorf("usage: /bye @agent [message]")
	}

	// Optional message
	message := ""
	if len(args) > 1 {
		message = strings.Join(args[1:], " ")
	}

	// Get agent from database
	agent, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: @%s", agentID)
	}

	now := time.Now().Unix()
	nowMs := time.Now().UnixMilli()

	// Clear claims
	clearedClaims, err := db.DeleteClaimsByAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear claims: %w", err)
	}

	// Clear session roles
	sessionRoles, err := db.GetSessionRoles(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session roles: %w", err)
	}
	clearedRoles, err := db.ClearSessionRoles(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear roles: %w", err)
	}
	for _, role := range sessionRoles {
		if err := db.AppendRoleStop(m.projectDBPath, agentID, role.RoleName, nowMs); err != nil {
			return nil, fmt.Errorf("failed to persist role stop: %w", err)
		}
	}

	// Handle wake condition lifecycle on bye
	clearedWake, err := db.ClearPersistUntilByeConditions(m.db, m.projectDBPath, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear wake conditions: %w", err)
	}
	pausedWake, err := db.PauseWakeConditions(m.db, m.projectDBPath, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to pause wake conditions: %w", err)
	}

	// Post optional message
	var posted *types.Message
	if message != "" {
		bases, err := db.GetAgentBases(m.db)
		if err != nil {
			return nil, err
		}
		mentions := core.ExtractMentions(message, bases)
		mentions = core.ExpandAllMention(mentions, bases)
		created, err := db.CreateMessage(m.db, types.Message{
			TS:        now,
			FromAgent: agentID,
			Body:      message,
			Mentions:  mentions,
		})
		if err != nil {
			return nil, err
		}
		if err := db.AppendMessage(m.projectDBPath, created); err != nil {
			return nil, err
		}
		posted = &created
	}

	// Post leave event
	eventMsg, err := db.CreateMessage(m.db, types.Message{
		TS:        now,
		FromAgent: agentID,
		Body:      fmt.Sprintf("@%s left", agentID),
		Type:      types.MessageTypeEvent,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, eventMsg); err != nil {
		return nil, err
	}

	// Update agent
	updates := db.AgentUpdates{
		LeftAt:   types.OptionalInt64{Set: true, Value: &now},
		LastSeen: types.OptionalInt64{Set: true, Value: &now},
		Status:   types.OptionalString{Set: true, Value: nil},
	}
	if err := db.UpdateAgent(m.db, agentID, updates); err != nil {
		return nil, err
	}

	// For managed agents, set presence to offline
	if agent.Managed {
		if err := db.UpdateAgentPresenceWithAudit(m.db, m.projectDBPath, agentID, agent.Presence, types.PresenceOffline, "bye", "chat", agent.Status); err != nil {
			return nil, err
		}
	}

	// Persist agent update
	updated, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, err
	}
	if updated != nil {
		if err := db.AppendAgent(m.projectDBPath, *updated); err != nil {
			return nil, err
		}
	}

	// Build status message
	var parts []string
	parts = append(parts, fmt.Sprintf("@%s left", agentID))
	if posted != nil {
		parts = append(parts, fmt.Sprintf("posted [%s]", posted.ID))
	}
	if clearedClaims > 0 {
		parts = append(parts, fmt.Sprintf("%d claims cleared", clearedClaims))
	}
	if clearedRoles > 0 {
		parts = append(parts, fmt.Sprintf("%d roles cleared", clearedRoles))
	}
	if clearedWake > 0 {
		parts = append(parts, fmt.Sprintf("%d wake cleared", clearedWake))
	}
	if pausedWake > 0 {
		parts = append(parts, fmt.Sprintf("%d wake paused", pausedWake))
	}

	m.status = strings.Join(parts, ", ")
	m.input.SetValue("")
	return nil, nil
}

// runFlyCommand spawns an offline agent with /fly skill context.
// Syntax: /fly @agent [message]
func (m *Model) runFlyCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /fly @agent [message]")
	}

	agentRef := args[0]
	agentID := strings.TrimPrefix(agentRef, "@")
	if agentID == "" {
		return nil, fmt.Errorf("usage: /fly @agent [message]")
	}

	// Get agent from database
	agent, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: @%s", agentID)
	}

	// State guard: agent must be offline
	if agent.Presence != types.PresenceOffline && agent.Presence != "" {
		return nil, fmt.Errorf("@%s is %s - run /bye @%s first", agentID, agent.Presence, agentID)
	}

	// Build trigger message
	now := time.Now().Unix()
	userMessage := ""
	if len(args) > 1 {
		userMessage = strings.Join(args[1:], " ")
	}

	// Post the trigger message: "@agent /fly"
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}
	triggerBody := fmt.Sprintf("@%s /fly", agentID)
	mentions := core.ExtractMentions(triggerBody, bases)
	mentions = core.ExpandAllMention(mentions, bases)

	triggerMsg, err := db.CreateMessage(m.db, types.Message{
		TS:        now,
		FromAgent: m.username,
		Body:      triggerBody,
		Mentions:  mentions,
		Type:      types.MessageTypeUser,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, triggerMsg); err != nil {
		return nil, err
	}

	// Post optional user message as a separate message that can be replied to
	if userMessage != "" {
		userMsgMentions := core.ExtractMentions(userMessage, bases)
		userMsgMentions = core.ExpandAllMention(userMsgMentions, bases)
		userMsg, err := db.CreateMessage(m.db, types.Message{
			TS:        now,
			FromAgent: m.username,
			Body:      userMessage,
			Mentions:  userMsgMentions,
			Type:      types.MessageTypeUser,
		})
		if err != nil {
			return nil, err
		}
		if err := db.AppendMessage(m.projectDBPath, userMsg); err != nil {
			return nil, err
		}
	}

	m.status = fmt.Sprintf("/fly @%s - daemon will spawn", agentID)
	m.input.SetValue("")

	// Reload messages to show the trigger
	if err := m.reloadMessages(); err != nil {
		return nil, err
	}

	return nil, nil
}

// runHopCommand spawns an offline agent with /hop skill context (auto-bye on idle).
// Syntax: /hop @agent [message]
func (m *Model) runHopCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /hop @agent [message]")
	}

	agentRef := args[0]
	agentID := strings.TrimPrefix(agentRef, "@")
	if agentID == "" {
		return nil, fmt.Errorf("usage: /hop @agent [message]")
	}

	// Get agent from database
	agent, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: @%s", agentID)
	}

	// State guard: agent must be offline or idle
	if agent.Presence != types.PresenceOffline && agent.Presence != types.PresenceIdle && agent.Presence != "" {
		return nil, fmt.Errorf("@%s is %s - run /bye @%s first", agentID, agent.Presence, agentID)
	}

	// Build trigger message
	now := time.Now().Unix()
	userMessage := ""
	if len(args) > 1 {
		userMessage = strings.Join(args[1:], " ")
	}

	// Post the trigger message: "@agent /hop"
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}
	triggerBody := fmt.Sprintf("@%s /hop", agentID)
	mentions := core.ExtractMentions(triggerBody, bases)
	mentions = core.ExpandAllMention(mentions, bases)

	triggerMsg, err := db.CreateMessage(m.db, types.Message{
		TS:        now,
		FromAgent: m.username,
		Body:      triggerBody,
		Mentions:  mentions,
		Type:      types.MessageTypeUser,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, triggerMsg); err != nil {
		return nil, err
	}

	// Post optional user message as a separate message that can be replied to
	if userMessage != "" {
		userMsgMentions := core.ExtractMentions(userMessage, bases)
		userMsgMentions = core.ExpandAllMention(userMsgMentions, bases)
		userMsg, err := db.CreateMessage(m.db, types.Message{
			TS:        now,
			FromAgent: m.username,
			Body:      userMessage,
			Mentions:  userMsgMentions,
			Type:      types.MessageTypeUser,
		})
		if err != nil {
			return nil, err
		}
		if err := db.AppendMessage(m.projectDBPath, userMsg); err != nil {
			return nil, err
		}
	}

	m.status = fmt.Sprintf("/hop @%s - daemon will spawn (auto-bye on idle)", agentID)
	m.input.SetValue("")

	// Reload messages to show the trigger
	if err := m.reloadMessages(); err != nil {
		return nil, err
	}

	return nil, nil
}

// runLandCommand asks an active/idle agent to run /land closeout.
// Syntax: /land @agent
func (m *Model) runLandCommand(args []string) (tea.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: /land @agent")
	}

	agentRef := args[0]
	agentID := strings.TrimPrefix(agentRef, "@")
	if agentID == "" {
		return nil, fmt.Errorf("usage: /land @agent")
	}

	// Get agent from database
	agent, err := db.GetAgent(m.db, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("agent not found: @%s", agentID)
	}

	// State guard: agent must be active or idle (has a running session)
	if agent.Presence != types.PresenceActive && agent.Presence != types.PresenceIdle &&
		agent.Presence != types.PresencePrompting && agent.Presence != types.PresencePrompted {
		return nil, fmt.Errorf("@%s is %s - nothing to land", agentID, agent.Presence)
	}

	// Post the trigger message: "@agent /land"
	now := time.Now().Unix()
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}
	triggerBody := fmt.Sprintf("@%s /land", agentID)
	mentions := core.ExtractMentions(triggerBody, bases)
	mentions = core.ExpandAllMention(mentions, bases)

	triggerMsg, err := db.CreateMessage(m.db, types.Message{
		TS:        now,
		FromAgent: m.username,
		Body:      triggerBody,
		Mentions:  mentions,
		Type:      types.MessageTypeUser,
	})
	if err != nil {
		return nil, err
	}
	if err := db.AppendMessage(m.projectDBPath, triggerMsg); err != nil {
		return nil, err
	}

	m.status = fmt.Sprintf("/land @%s - asked to run /land", agentID)
	m.input.SetValue("")

	// Reload messages to show the trigger
	if err := m.reloadMessages(); err != nil {
		return nil, err
	}

	return nil, nil
}
