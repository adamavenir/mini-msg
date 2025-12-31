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
	case "/edit":
		return nil, m.runEditCommand(input)
	case "/delete", "/rm":
		return nil, m.runDeleteCommand(input)
	case "/prune":
		return nil, m.runPruneCommand(fields[1:])
	}

	return nil, fmt.Errorf("unknown command: %s", fields[0])
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
			helpItemStyle+"Tab"+helpResetStyle+" - thread/channel list",
			25,
			22,
		),
		"",
		helpLabelStyle + "Commands" + helpResetStyle,
		formatHelpRow(
			helpItemStyle+"/edit <id> <text> -m <reason>"+helpResetStyle,
			helpItemStyle+"/delete <id>"+helpResetStyle,
			helpItemStyle+"/prune [--keep N]"+helpResetStyle,
			25,
			22,
		),
		formatHelpRow(
			helpItemStyle+"/help"+helpResetStyle,
			helpItemStyle+"/quit"+helpResetStyle,
			"",
			25,
			22,
		),
		"",
		helpItemStyle + "Click" + helpResetStyle + " a message to start a threaded reply. " +
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
