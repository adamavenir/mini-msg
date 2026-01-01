package chat

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) renderMessages() string {
	if m.currentPseudo != "" {
		return m.renderQuestions()
	}
	messages := m.currentMessages()
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)

	// Build read_to map: message_guid -> list of agents
	home := "room"
	if m.currentThread != nil {
		home = m.currentThread.GUID
	}
	readToMap := make(map[string][]string)
	if positions, err := db.GetReadToForHome(m.db, home); err == nil {
		for _, pos := range positions {
			readToMap[pos.MessageGUID] = append(readToMap[pos.MessageGUID], pos.AgentID)
		}
	}

	chunks := make([]string, 0, len(messages))
	for _, msg := range messages {
		chunks = append(chunks, m.formatMessage(msg, prefixLength, readToMap))
	}
	return strings.Join(chunks, "\n\n")
}

func (m *Model) formatMessage(msg types.Message, prefixLength int, readToMap map[string][]string) string {
	if msg.Type == types.MessageTypeEvent {
		body := msg.Body
		hasANSI := strings.Contains(body, "\x1b[")
		width := m.mainWidth()
		if width > 0 {
			body = ansi.Wrap(body, width, "")
		}
		if hasANSI {
			return body
		}
		style := lipgloss.NewStyle().Foreground(metaColor).Italic(true)
		return style.Render(body)
	}

	color := userColor
	if msg.Type != types.MessageTypeUser {
		color = colorForAgent(msg.FromAgent, m.colorMap)
	}

	sender := renderByline(msg.FromAgent, color)
	strippedBody := core.StripQuestionSections(msg.Body)
	body := highlightCodeBlocks(strippedBody)
	width := m.mainWidth()
	if width > 0 {
		body = ansi.Wrap(body, width, "")
	}
	bodyLine := lipgloss.NewStyle().Foreground(color).Render(body)
	editedSuffix := ""
	if msg.Edited || msg.EditCount > 0 || msg.EditedAt != nil {
		editedSuffix = " (edited)"
	}

	// Build the meta line with guid and read_to markers
	guidPart := fmt.Sprintf("#%s%s", core.GetGUIDPrefix(msg.ID, prefixLength), editedSuffix)
	readToPart := ""
	if agents, ok := readToMap[msg.ID]; ok && len(agents) > 0 {
		mentions := make([]string, len(agents))
		for i, agent := range agents {
			mentions[i] = "@" + agent
		}
		readToPart = "read_to: " + strings.Join(mentions, " ")
	}

	var meta string
	if readToPart != "" && width > 0 {
		// Right-align read_to on the same line
		guidWidth := len(guidPart)
		readWidth := len(readToPart)
		padding := width - guidWidth - readWidth
		if padding < 2 {
			padding = 2
		}
		metaText := guidPart + strings.Repeat(" ", padding) + readToPart
		meta = lipgloss.NewStyle().Foreground(color).Faint(true).Render(metaText)
	} else if readToPart != "" {
		meta = lipgloss.NewStyle().Foreground(color).Faint(true).Render(guidPart + "  " + readToPart)
	} else {
		meta = lipgloss.NewStyle().Foreground(color).Faint(true).Render(guidPart)
	}

	lines := []string{}
	if msg.ReplyTo != nil {
		lines = append(lines, m.replyContext(*msg.ReplyTo, prefixLength))
	}
	lines = append(lines, fmt.Sprintf("%s\n%s", sender, bodyLine))
	if reactionLine := formatReactionSummary(msg.Reactions); reactionLine != "" {
		line := lipgloss.NewStyle().Foreground(metaColor).Faint(true).Render(reactionLine)
		if width > 0 {
			line = ansi.Wrap(line, width, "")
		}
		lines = append(lines, line)
	}
	lines = append(lines, meta)
	return strings.Join(lines, "\n")
}

func (m *Model) replyContext(replyTo string, prefixLength int) string {
	row := m.db.QueryRow(`
		SELECT from_agent, body FROM fray_messages WHERE guid = ?
	`, replyTo)
	var fromAgent string
	var body string
	if err := row.Scan(&fromAgent, &body); err != nil {
		prefix := core.GetGUIDPrefix(replyTo, prefixLength)
		return lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("↪ Reply to #%s", prefix))
	}
	preview := truncatePreview(body, 50)
	return lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("↪ Reply to @%s: %s", fromAgent, preview))
}

func renderByline(agent string, color lipgloss.Color) string {
	content := fmt.Sprintf(" @%s: ", agent)
	textColor := contrastTextColor(color)
	style := lipgloss.NewStyle().Background(color).Foreground(textColor).Bold(true)
	return style.Render(content)
}

func formatReactionSummary(reactions map[string][]string) string {
	if len(reactions) == 0 {
		return ""
	}
	keys := make([]string, 0, len(reactions))
	for reaction := range reactions {
		keys = append(keys, reaction)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, reaction := range keys {
		users := uniqueSortedStrings(reactions[reaction])
		if len(users) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s", reaction, strings.Join(users, ", ")))
	}
	return strings.Join(parts, " · ")
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func diffReactions(before, after map[string][]string) map[string][]string {
	added := map[string][]string{}
	beforeSets := reactionSets(before)
	for reaction, users := range after {
		previous := beforeSets[reaction]
		for _, user := range users {
			if _, ok := previous[user]; ok {
				continue
			}
			added[reaction] = append(added[reaction], user)
		}
	}
	return added
}

func reactionsEqual(left, right map[string][]string) bool {
	if len(left) != len(right) {
		return false
	}
	leftSets := reactionSets(left)
	rightSets := reactionSets(right)
	if len(leftSets) != len(rightSets) {
		return false
	}
	for reaction, leftUsers := range leftSets {
		rightUsers, ok := rightSets[reaction]
		if !ok || len(leftUsers) != len(rightUsers) {
			return false
		}
		for user := range leftUsers {
			if _, ok := rightUsers[user]; !ok {
				return false
			}
		}
	}
	return true
}

func reactionSets(values map[string][]string) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(values))
	for reaction, users := range values {
		set := map[string]struct{}{}
		for _, user := range users {
			if user == "" {
				continue
			}
			set[user] = struct{}{}
		}
		out[reaction] = set
	}
	return out
}

func newEventMessage(body string) types.Message {
	return types.Message{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		TS:        time.Now().Unix(),
		FromAgent: "system",
		Body:      body,
		Type:      types.MessageTypeEvent,
	}
}

func filterUpdates(messages []types.Message, showUpdates bool) []types.Message {
	if showUpdates {
		return messages
	}
	filtered := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Type == types.MessageTypeEvent {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

func (m *Model) messageAtLine(line int) (*types.Message, bool) {
	if line < 0 {
		return nil, false
	}
	if m.currentPseudo != "" {
		return nil, false
	}
	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	cursor := 0
	messages := m.currentMessages()
	emptyReadTo := map[string][]string{}
	for i, msg := range messages {
		formatted := m.formatMessage(msg, prefixLength, emptyReadTo)
		lines := lipgloss.Height(formatted)
		if line >= cursor && line < cursor+lines {
			if msg.Type == types.MessageTypeEvent {
				return nil, true
			}
			return &messages[i], true
		}
		cursor += lines
		if i < len(messages)-1 {
			if line == cursor {
				return nil, true
			}
			cursor++
		}
	}
	return nil, false
}

func truncatePreview(body string, maxLen int) string {
	compact := strings.Join(strings.Fields(body), " ")
	if len(compact) <= maxLen {
		return compact
	}
	return compact[:maxLen-3] + "..."
}

func truncateLine(value string, maxLen int) string {
	if maxLen <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}
