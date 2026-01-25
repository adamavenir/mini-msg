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
)

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
	// Explicitly reset to normal text color (don't rely on conditional logic)
	m.wasEditMode = false
	m.reactionMode = false
	m.replyMode = false
	applyInputStyles(&m.input, textColor, blurText)
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

	deletedAt := int64(0)
	if updated.ArchivedAt != nil {
		deletedAt = *updated.ArchivedAt
	}
	var deletedBy *string
	if m.username != "" {
		deletedBy = &m.username
	}
	if err := db.AppendMessageDelete(m.projectDBPath, updated.ID, deletedBy, deletedAt); err != nil {
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
	keep, pruneAll, target, withReact, withFlags, withoutFlags, err := parsePruneArgs(args)
	if err != nil {
		return err
	}

	opts := parsePruneProtectionOpts(withFlags, withoutFlags)

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

	root := projectRootFromPath(m.projectDBPath)
	if err := checkPruneGuardrails(root); err != nil {
		return err
	}

	var result pruneResult
	if withReact != "" {
		result, err = pruneMessagesWithReaction(m.projectDBPath, home, withReact)
	} else {
		result, err = pruneMessages(m.projectDBPath, keep, pruneAll, home, opts)
	}
	if err != nil {
		return err
	}

	if err := db.RebuildDatabaseFromJSONL(m.db, m.projectDBPath); err != nil {
		return err
	}

	// Fix stale watermarks pointing to pruned messages
	_ = fixStaleWatermarks(m.db, m.projectDBPath)

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
	m.hasMore = len(rawMessages) >= m.lastLimit
	m.colorMap = colorMap
	m.refreshViewport(false)
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

func parsePruneArgs(args []string) (int, bool, string, string, []string, []string, error) {
	keep := 20
	pruneAll := false
	target := ""
	withReact := ""
	var withFlags []string
	var withoutFlags []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--all":
			pruneAll = true
		case arg == "--keep":
			if i+1 >= len(args) {
				return 0, false, "", "", nil, nil, fmt.Errorf("usage: /prune [target] [--keep N] [--all] [--with X] [--without X]")
			}
			i++
			value, err := parseNonNegativeInt(args[i])
			if err != nil {
				return 0, false, "", "", nil, nil, err
			}
			keep = value
		case strings.HasPrefix(arg, "--keep="):
			value, err := parseNonNegativeInt(strings.TrimPrefix(arg, "--keep="))
			if err != nil {
				return 0, false, "", "", nil, nil, err
			}
			keep = value
		case arg == "--with-react":
			if i+1 >= len(args) {
				return 0, false, "", "", nil, nil, fmt.Errorf("usage: /prune [target] [--with-react emoji]")
			}
			i++
			withReact = args[i]
		case strings.HasPrefix(arg, "--with-react="):
			withReact = strings.TrimPrefix(arg, "--with-react=")
		case arg == "--with":
			if i+1 >= len(args) {
				return 0, false, "", "", nil, nil, fmt.Errorf("usage: /prune [target] [--with replies,faves,reacts]")
			}
			i++
			withFlags = append(withFlags, args[i])
		case strings.HasPrefix(arg, "--with="):
			withFlags = append(withFlags, strings.TrimPrefix(arg, "--with="))
		case arg == "--without":
			if i+1 >= len(args) {
				return 0, false, "", "", nil, nil, fmt.Errorf("usage: /prune [target] [--without replies,faves,reacts]")
			}
			i++
			withoutFlags = append(withoutFlags, args[i])
		case strings.HasPrefix(arg, "--without="):
			withoutFlags = append(withoutFlags, strings.TrimPrefix(arg, "--without="))
		case strings.HasPrefix(arg, "--"):
			return 0, false, "", "", nil, nil, fmt.Errorf("unknown flag: %s", arg)
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
				return 0, false, "", "", nil, nil, fmt.Errorf("usage: /prune [target] [--keep N] [--all] [--with X] [--without X]")
			}
		}
	}

	return keep, pruneAll, target, withReact, withFlags, withoutFlags, nil
}

func parseNonNegativeInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid value: %s", value)
	}
	return parsed, nil
}
