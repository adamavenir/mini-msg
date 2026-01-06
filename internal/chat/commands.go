package chat

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	helpLabelStyle = "\x1b[1m\x1b[97m"
	helpItemStyle  = "\x1b[94m"
	helpResetStyle = "\x1b[0m"
)

func (m *Model) handleSlashCommand(input string) (bool, tea.Cmd) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
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
	case "/mv":
		// Move current thread to new parent
		return m.runMvCommand(fields[1:])
	case "/edit":
		return nil, m.runEditCommand(input)
	case "/delete", "/rm":
		return nil, m.runDeleteCommand(input)
	case "/prune":
		return nil, fmt.Errorf("/prune is disabled (see fray-jqxf)")
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
		// Fall back to focused thread
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
	} else {
		return nil, fmt.Errorf("/n only works when thread panel is focused (press Tab) or via Ctrl-N")
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

	body := strings.ReplaceAll(msg.Body, "\n", " ")
	value := fmt.Sprintf("/edit #%s %s -m ", msg.ID, body)
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.clearSuggestions()
	m.lastInputValue = m.input.Value()
	m.lastInputPos = m.inputCursorPos()
	m.status = ""
	return true
}

func (m *Model) lastUserMessage() *types.Message {
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
	keep, pruneAll, err := parsePruneArgs(args)
	if err != nil {
		return err
	}

	root := projectRootFromPath(m.projectDBPath)
	if err := checkPruneGuardrails(root); err != nil {
		return err
	}

	result, err := pruneMessages(m.projectDBPath, keep, pruneAll)
	if err != nil {
		return err
	}

	if err := db.RebuildDatabaseFromJSONL(m.db, m.projectDBPath); err != nil {
		return err
	}

	if err := m.reloadMessages(); err != nil {
		return err
	}

	if result.ClearedHistory {
		m.status = fmt.Sprintf("Pruned to last %d messages. history.jsonl cleared.", result.Kept)
		return nil
	}

	m.status = fmt.Sprintf("Pruned to last %d messages. Archived to history.jsonl", result.Kept)
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

func parsePruneArgs(args []string) (int, bool, error) {
	keep := 20
	pruneAll := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--all":
			pruneAll = true
		case arg == "--keep":
			if i+1 >= len(args) {
				return 0, false, fmt.Errorf("usage: /prune [--keep N] [--all]")
			}
			i++
			value, err := parseNonNegativeInt(args[i])
			if err != nil {
				return 0, false, err
			}
			keep = value
		case strings.HasPrefix(arg, "--keep="):
			value, err := parseNonNegativeInt(strings.TrimPrefix(arg, "--keep="))
			if err != nil {
				return 0, false, err
			}
			keep = value
		default:
			value, err := parseNonNegativeInt(arg)
			if err != nil {
				return 0, false, fmt.Errorf("usage: /prune [--keep N] [--all]")
			}
			keep = value
		}
	}

	return keep, pruneAll, nil
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

	// Try name match
	thread, err = db.GetThreadByName(m.db, value, nil)
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
