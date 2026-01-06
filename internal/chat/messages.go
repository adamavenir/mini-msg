package chat

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// inlineIDPattern matches #-prefixed IDs like #fray-abc123, #msg-xyz789, #abc123
// Format: # followed by either:
//   - prefix-id: word followed by dash and alphanumeric (e.g., #fray-abc123)
//   - short id: alphanumeric with at least one letter (e.g., #abc123, #a1b2)
var inlineIDPattern = regexp.MustCompile(`#([a-zA-Z]+-[a-z0-9]+|[a-z0-9]*[a-z][a-z0-9]*)`)

func (m *Model) renderMessages() string {
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

	// Get avatar for agent, or human avatar for users
	avatar := m.avatarMap[msg.FromAgent]
	if msg.Type == types.MessageTypeUser && avatar == "" {
		avatar = core.HumanAvatar
	}

	// Mark byline as zone for copying whole message
	bylineText := renderByline(msg.FromAgent, avatar, color)
	sender := m.zoneManager.Mark("byline-"+msg.ID, bylineText)

	body := highlightCodeBlocks(msg.Body)
	width := m.mainWidth()
	if width > 0 {
		body = ansi.Wrap(body, width, "")
	}

	// Parse body into paragraphs and mark each as a zone
	bodyLine := m.markBodyZones(msg.ID, body, color)
	editedSuffix := ""
	if msg.Edited || msg.EditCount > 0 || msg.EditedAt != nil {
		editedSuffix = " (edited)"
	}

	// Build the meta line with guid and read_to markers
	guidPrefix := core.GetGUIDPrefix(msg.ID, prefixLength)
	guidText := fmt.Sprintf("#%s%s", guidPrefix, editedSuffix)
	// Footer GUID stays dimmed (no bold/underline) but is click-to-copy zone
	guidPart := m.zoneManager.Mark("guid-"+msg.ID, guidText)

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
		// Note: use unstyled guidText for width calculation
		guidWidth := len(guidText)
		readWidth := len(readToPart)
		padding := width - guidWidth - readWidth
		if padding < 2 {
			padding = 2
		}
		metaText := guidPart + strings.Repeat(" ", padding) + readToPart
		styledMeta := lipgloss.NewStyle().Foreground(color).Faint(true).Render(metaText)
		meta = m.zoneManager.Mark("footer-"+msg.ID, styledMeta)
	} else if readToPart != "" {
		styledMeta := lipgloss.NewStyle().Foreground(color).Faint(true).Render(guidPart + "  " + readToPart)
		meta = m.zoneManager.Mark("footer-"+msg.ID, styledMeta)
	} else {
		styledMeta := lipgloss.NewStyle().Foreground(color).Faint(true).Render(guidPart)
		meta = m.zoneManager.Mark("footer-"+msg.ID, styledMeta)
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
	if questionLine := m.formatQuestionStatus(msg.ID); questionLine != "" {
		if width > 0 {
			questionLine = ansi.Wrap(questionLine, width, "")
		}
		lines = append(lines, questionLine)
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

func (m *Model) formatQuestionStatus(msgID string) string {
	questions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		AskedIn: &msgID,
	})
	if err != nil || len(questions) == 0 {
		return ""
	}

	var answered, unanswered []string
	for i, q := range questions {
		label := fmt.Sprintf("Q%d", i+1)
		if q.Status == types.QuestionStatusAnswered {
			answered = append(answered, label)
		} else {
			unanswered = append(unanswered, label)
		}
	}

	var parts []string
	if len(answered) > 0 {
		answeredStyle := lipgloss.NewStyle().Bold(true)
		parts = append(parts, answeredStyle.Render("Answered")+": "+strings.Join(answered, ", "))
	}
	if len(unanswered) > 0 {
		unansweredStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")) // yellow
		parts = append(parts, unansweredStyle.Render("Unanswered")+": "+strings.Join(unanswered, ", "))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "  ")
}

func (m *Model) markBodyZones(msgID string, body string, color lipgloss.Color) string {
	lines := strings.Split(body, "\n")
	textStyle := lipgloss.NewStyle().Foreground(color)
	idStyle := lipgloss.NewStyle().Foreground(color).Bold(true).Underline(true)
	styledLines := make([]string, len(lines))

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			styledLines[i] = line
			continue
		}

		// Find inline IDs and style them specially
		styledLine := m.styleLineWithInlineIDs(msgID, i, line, textStyle, idStyle)
		zoneID := fmt.Sprintf("line-%s-%d", msgID, i)
		styledLines[i] = m.zoneManager.Mark(zoneID, styledLine)
	}

	return strings.Join(styledLines, "\n")
}

// styleLineWithInlineIDs finds #-prefixed IDs in a line and styles them as bold+underline zones
func (m *Model) styleLineWithInlineIDs(msgID string, lineNum int, line string, textStyle, idStyle lipgloss.Style) string {
	matches := inlineIDPattern.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return textStyle.Render(line)
	}

	var result strings.Builder
	cursor := 0

	for idx, match := range matches {
		// Add text before this match
		if match[0] > cursor {
			result.WriteString(textStyle.Render(line[cursor:match[0]]))
		}

		// Style and zone the ID
		idText := line[match[0]:match[1]]
		styledID := idStyle.Render(idText)
		zoneID := fmt.Sprintf("inlineid-%s-%d-%d", msgID, lineNum, idx)
		result.WriteString(m.zoneManager.Mark(zoneID, styledID))

		cursor = match[1]
	}

	// Add remaining text after last match
	if cursor < len(line) {
		result.WriteString(textStyle.Render(line[cursor:]))
	}

	return result.String()
}

func renderByline(agent string, avatar string, color lipgloss.Color) string {
	var content string
	if avatar != "" {
		content = fmt.Sprintf(" %s @%s: ", avatar, agent)
	} else {
		content = fmt.Sprintf(" @%s: ", agent)
	}
	textColor := contrastTextColor(color)
	style := lipgloss.NewStyle().Background(color).Foreground(textColor).Bold(true)
	return style.Render(content)
}

func formatReactionSummary(reactions map[string][]types.ReactionEntry) string {
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
		entries := reactions[reaction]
		count := len(entries)
		if count == 0 {
			continue
		}
		if count == 1 {
			// Single reaction: show "👍 alice"
			parts = append(parts, fmt.Sprintf("%s %s", reaction, entries[0].AgentID))
		} else {
			// Multiple reactions: show "👍x3"
			parts = append(parts, fmt.Sprintf("%sx%d", reaction, count))
		}
	}
	return strings.Join(parts, " · ")
}

func diffReactions(before, after map[string][]types.ReactionEntry) map[string][]types.ReactionEntry {
	added := map[string][]types.ReactionEntry{}
	for reaction, entries := range after {
		beforeEntries := before[reaction]
		beforeSet := make(map[string]int64) // agent -> max timestamp seen
		for _, e := range beforeEntries {
			if e.ReactedAt > beforeSet[e.AgentID] {
				beforeSet[e.AgentID] = e.ReactedAt
			}
		}
		for _, e := range entries {
			// Consider "added" if this timestamp is newer than what we saw before
			if prevTs, ok := beforeSet[e.AgentID]; !ok || e.ReactedAt > prevTs {
				added[reaction] = append(added[reaction], e)
			}
		}
	}
	return added
}

func reactionsEqual(left, right map[string][]types.ReactionEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for reaction, leftEntries := range left {
		rightEntries, ok := right[reaction]
		if !ok || len(leftEntries) != len(rightEntries) {
			return false
		}
		// For equality, we check that the sets of (agent, timestamp) are the same
		leftSet := make(map[string]int64)
		for _, e := range leftEntries {
			leftSet[fmt.Sprintf("%s:%d", e.AgentID, e.ReactedAt)] = 1
		}
		for _, e := range rightEntries {
			key := fmt.Sprintf("%s:%d", e.AgentID, e.ReactedAt)
			if _, ok := leftSet[key]; !ok {
				return false
			}
		}
	}
	return true
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
