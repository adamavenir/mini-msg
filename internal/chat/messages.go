package chat

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// inlineIDPattern matches #-prefixed IDs like #fray-abc123, #msg-xyz789, #abc123
// Format: # followed by either:
//   - prefix-id: word followed by dash and alphanumeric (e.g., #fray-abc123)
//   - prefix-id with suffix: includes .n suffix when followed by non-space (e.g., #fray-abc123.1)
//   - short id: alphanumeric with at least one letter (e.g., #abc123, #a1b2)
// Note: The .n suffix is included only when followed by a non-space character (e.g., #fray-abc.1)
// but not when the period starts a new sentence (e.g., "#fray-abc. New sentence")
var inlineIDPattern = regexp.MustCompile(`#([a-zA-Z]+-[a-z0-9]+(?:\.[a-z0-9]+)?|[a-z0-9]*[a-z][a-z0-9]*)`)

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

	// Get anchor message GUID if viewing a thread
	var anchorGUID string
	if m.currentThread != nil && m.currentThread.AnchorMessageGUID != nil {
		anchorGUID = *m.currentThread.AnchorMessageGUID
	}

	chunks := make([]string, 0, len(messages))
	for _, msg := range messages {
		chunks = append(chunks, m.formatMessage(msg, prefixLength, readToMap))

		// After anchor message, insert subthread tree preview
		if anchorGUID != "" && msg.ID == anchorGUID {
			if tree := m.renderSubthreadTree(); tree != "" {
				chunks = append(chunks, tree)
			}
		}
	}
	return strings.Join(chunks, "\n\n")
}

func (m *Model) formatMessage(msg types.Message, prefixLength int, readToMap map[string][]string) string {
	if msg.Type == types.MessageTypeEvent {
		// Check for interactive event
		if event := parseInteractiveEvent(msg); event != nil {
			return m.renderInteractiveEvent(msg, event, m.mainWidth())
		}

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

	// Tombstone messages get distinct styling - dimmed with border
	if msg.Type == types.MessageTypeTombstone {
		return m.formatTombstone(msg, prefixLength)
	}

	color := userColor
	if msg.Type != types.MessageTypeUser {
		color = colorForAgent(msg.FromAgent, m.colorMap)
	}

	// Mark byline as zone for copying whole message
	// Avatar rendering disabled per fray-i2cq (deemed too busy)
	bylineText := renderByline(m.displayAgentLabel(msg), "", color)
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

	// Build the meta line with guid, session_id (optional), and read_to markers
	guidPrefix := core.GetGUIDPrefix(msg.ID, prefixLength)
	guidText := fmt.Sprintf("#%s%s", guidPrefix, editedSuffix)
	// Footer GUID stays dimmed (no bold/underline) but is click-to-copy zone
	guidPart := m.zoneManager.Mark("guid-"+msg.ID, guidText)

	// Add session ID to footer if present (abbreviated to first 8 chars)
	sessionPart := ""
	if msg.SessionID != nil && *msg.SessionID != "" {
		sessID := *msg.SessionID
		if len(sessID) > 8 {
			sessID = sessID[:8]
		}
		sessionPart = fmt.Sprintf("sess:%s", sessID)
	}

	readToPart := ""
	if agents, ok := readToMap[msg.ID]; ok && len(agents) > 0 {
		mentions := make([]string, len(agents))
		for i, agent := range agents {
			mentions[i] = "@" + agent
		}
		readToPart = "read_to: " + strings.Join(mentions, " ")
	}

	// Build left side of footer: guid + session
	leftParts := []string{guidPart}
	if sessionPart != "" {
		leftParts = append(leftParts, sessionPart)
	}
	leftText := strings.Join(leftParts, "  ")
	leftWidth := len(guidText)
	if sessionPart != "" {
		leftWidth += 2 + len(sessionPart)
	}

	var meta string
	if readToPart != "" && width > 0 {
		// Right-align read_to on the same line
		readWidth := len(readToPart)
		padding := width - leftWidth - readWidth
		if padding < 2 {
			padding = 2
		}
		metaText := leftText + strings.Repeat(" ", padding) + readToPart
		styledMeta := lipgloss.NewStyle().Foreground(color).Faint(true).Render(metaText)
		meta = m.zoneManager.Mark("footer-"+msg.ID, styledMeta)
	} else if readToPart != "" {
		styledMeta := lipgloss.NewStyle().Foreground(color).Faint(true).Render(leftText + "  " + readToPart)
		meta = m.zoneManager.Mark("footer-"+msg.ID, styledMeta)
	} else {
		styledMeta := lipgloss.NewStyle().Foreground(color).Faint(true).Render(leftText)
		meta = m.zoneManager.Mark("footer-"+msg.ID, styledMeta)
	}

	lines := []string{}
	if msg.ReplyTo != nil {
		lines = append(lines, m.replyContext(*msg.ReplyTo, prefixLength))
	}
	lines = append(lines, fmt.Sprintf("%s\n%s", sender, bodyLine))
	if reactionLine := formatReactionSummary(msg.Reactions); reactionLine != "" {
		// Don't wrap with additional styling - formatReactionSummary handles styling internally
		if width > 0 {
			reactionLine = ansi.Wrap(reactionLine, width, "")
		}
		lines = append(lines, reactionLine)
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

func (m *Model) displayAgentLabel(msg types.Message) string {
	label := msg.FromAgent
	if msg.Origin == "" {
		return label
	}
	if m.agentHasMultipleOrigins(msg.FromAgent, msg.Origin) {
		return fmt.Sprintf("%s@%s", msg.FromAgent, msg.Origin)
	}
	return label
}

func (m *Model) agentHasMultipleOrigins(agentID, origin string) bool {
	if agentID == "" {
		return false
	}
	if m.agentOrigins == nil {
		m.agentOrigins = make(map[string]map[string]struct{})
	}
	origins, ok := m.agentOrigins[agentID]
	if !ok {
		list, err := db.GetDistinctOriginsForAgent(m.db, agentID)
		origins = make(map[string]struct{}, len(list))
		if err == nil {
			for _, item := range list {
				if item == "" {
					continue
				}
				origins[item] = struct{}{}
			}
		}
	}
	if origin != "" {
		origins[origin] = struct{}{}
	}
	m.agentOrigins[agentID] = origins
	return len(origins) > 1
}

func (m *Model) replyContext(replyTo string, prefixLength int) string {
	row := m.db.QueryRow(`
		SELECT from_agent, origin, body FROM fray_messages WHERE guid = ?
	`, replyTo)
	var fromAgent string
	var origin sql.NullString
	var body string
	if err := row.Scan(&fromAgent, &origin, &body); err != nil {
		prefix := core.GetGUIDPrefix(replyTo, prefixLength)
		return lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("↪ Reply to #%s", prefix))
	}
	display := m.displayAgentLabel(types.Message{FromAgent: fromAgent, Origin: origin.String})
	preview := truncatePreview(body, 50)
	return lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("↪ Reply to @%s: %s", display, preview))
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

	// Pill styling: dim grey background with padding
	pillBg := lipgloss.Color("236")   // dim grey bg
	pillPadStyle := lipgloss.NewStyle().Background(pillBg).Padding(0, 1)
	// Yellow for text reactions (bold to ensure visibility on dark bg)
	textStyle := lipgloss.NewStyle().Foreground(reactionColor).Background(pillBg).Bold(true)
	// Regular text for emoji reactions
	emojiStyle := lipgloss.NewStyle().Background(pillBg)
	// Dim style for signoff
	signoffStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Background(pillBg)

	// Tree connector to show reactions are "attached" - use terminal connector
	// since reactions are the last item before the footer
	treeBar := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("└─")

	pills := make([]string, 0, len(keys))
	for _, reaction := range keys {
		entries := reactions[reaction]
		count := len(entries)
		if count == 0 {
			continue
		}

		var pillContent string
		if count == 1 {
			// Single reaction: "reaction --@agent"
			agent := entries[0].AgentID
			if isTextReaction(reaction) {
				pillContent = textStyle.Render(reaction) + signoffStyle.Render(" --@"+agent)
			} else {
				pillContent = emojiStyle.Render(reaction) + signoffStyle.Render(" --@"+agent)
			}
		} else {
			// Multiple reactions: "reaction x3"
			if isTextReaction(reaction) {
				pillContent = textStyle.Render(reaction) + signoffStyle.Render(fmt.Sprintf(" x%d", count))
			} else {
				pillContent = emojiStyle.Render(reaction) + signoffStyle.Render(fmt.Sprintf(" x%d", count))
			}
		}

		// Wrap in pill padding (background already applied to content)
		pills = append(pills, pillPadStyle.Render(pillContent))
	}

	return treeBar + " " + strings.Join(pills, " ")
}

// isTextReaction returns true if the reaction is plain text (not emoji)
// Text reactions are ASCII letters/numbers/common punctuation
func isTextReaction(reaction string) bool {
	for _, r := range reaction {
		// If any rune is outside the basic ASCII printable range, it's likely emoji
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return false
		}
	}
	return len(reaction) > 0
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
		// Filter out all event messages when showUpdates is false
		if msg.Type == types.MessageTypeEvent {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

// isJoinLeaveEvent returns true if the message is a join/leave/rejoin event
func isJoinLeaveEvent(msg types.Message) bool {
	if msg.Type != types.MessageTypeEvent {
		return false
	}
	body := msg.Body
	// Check for the standard join/leave/rejoin patterns
	// These are: "@agent joined", "@agent rejoined", "@agent left"
	if strings.HasPrefix(body, "@") {
		if strings.HasSuffix(body, " joined") ||
			strings.HasSuffix(body, " rejoined") ||
			strings.HasSuffix(body, " left") {
			return true
		}
	}
	return false
}

// filterJoinLeaveEvents removes join/leave event messages while keeping other events
func filterJoinLeaveEvents(messages []types.Message) []types.Message {
	filtered := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if isJoinLeaveEvent(msg) {
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

	// Build read_to map (same as renderMessages to ensure consistent line heights)
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

	// Get anchor message GUID if viewing a thread (for subthread tree rendering)
	var anchorGUID string
	if m.currentThread != nil && m.currentThread.AnchorMessageGUID != nil {
		anchorGUID = *m.currentThread.AnchorMessageGUID
	}

	for i, msg := range messages {
		formatted := m.formatMessage(msg, prefixLength, readToMap)
		lines := lipgloss.Height(formatted)
		if line >= cursor && line < cursor+lines {
			if msg.Type == types.MessageTypeEvent {
				return nil, true
			}
			return &messages[i], true
		}
		cursor += lines

		// Account for subthread tree after anchor message (same as renderMessages)
		if anchorGUID != "" && msg.ID == anchorGUID {
			if tree := m.renderSubthreadTree(); tree != "" {
				treeLines := lipgloss.Height(tree)
				if line >= cursor && line < cursor+treeLines {
					return nil, true // clicked on tree, not a message
				}
				cursor += treeLines
			}
		}

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

// formatTombstone renders a tombstone message with distinct styling.
// Tombstones indicate pruned message ranges and use a dimmed, bordered style.
func (m *Model) formatTombstone(msg types.Message, prefixLength int) string {
	width := m.mainWidth()

	// Style the body with inline ID highlighting
	idStyle := lipgloss.NewStyle().Foreground(metaColor).Bold(true).Underline(true)
	textStyle := lipgloss.NewStyle().Foreground(metaColor).Faint(true)

	// Find and style #msg-* IDs in the body
	body := m.styleTombstoneBody(msg.ID, msg.Body, textStyle, idStyle)

	if width > 0 {
		body = ansi.Wrap(body, width-4, "") // Account for border
	}

	// Add tombstone icon and GUID footer
	guidPrefix := core.GetGUIDPrefix(msg.ID, prefixLength)
	guidText := fmt.Sprintf("#%s", guidPrefix)
	guidPart := m.zoneManager.Mark("guid-"+msg.ID, textStyle.Render(guidText))

	// Build the tombstone with a subtle left border
	borderStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(metaColor).
		PaddingLeft(1).
		Foreground(metaColor).
		Faint(true)

	content := fmt.Sprintf("⏏︎ %s\n%s", body, guidPart)
	return borderStyle.Render(content)
}

// styleTombstoneBody finds #msg-* IDs in tombstone body and styles them as clickable zones
func (m *Model) styleTombstoneBody(msgID, body string, textStyle, idStyle lipgloss.Style) string {
	matches := inlineIDPattern.FindAllStringIndex(body, -1)
	if len(matches) == 0 {
		return textStyle.Render(body)
	}

	var result strings.Builder
	cursor := 0

	for idx, match := range matches {
		// Add text before this match
		if match[0] > cursor {
			result.WriteString(textStyle.Render(body[cursor:match[0]]))
		}

		// Style and zone the ID
		idText := body[match[0]:match[1]]
		styledID := idStyle.Render(idText)
		zoneID := fmt.Sprintf("tombstone-id-%s-%d", msgID, idx)
		result.WriteString(m.zoneManager.Mark(zoneID, styledID))

		cursor = match[1]
	}

	// Add remaining text after last match
	if cursor < len(body) {
		result.WriteString(textStyle.Render(body[cursor:]))
	}

	return result.String()
}

// renderSubthreadTree renders a tree preview of child threads under the anchor.
// Shows immediate children with message counts and activity indicators.
func (m *Model) renderSubthreadTree() string {
	if m.currentThread == nil {
		return ""
	}

	// Get child threads with stats
	children, err := db.GetChildThreadsWithStats(m.db, m.currentThread.GUID)
	if err != nil || len(children) == 0 {
		return ""
	}

	// Limit to first 5 children to keep it compact
	maxChildren := 5
	if len(children) > maxChildren {
		children = children[:maxChildren]
	}

	// Build tree lines
	treeStyle := lipgloss.NewStyle().Foreground(metaColor)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(metaColor).Faint(true)
	activityStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("35"))

	var lines []string
	for i, child := range children {
		// Tree branch character
		branch := "├─"
		if i == len(children)-1 {
			branch = "└─"
		}

		// Activity indicator (recent = within 1 hour)
		activityIndicator := ""
		if child.LastActivityAt != nil {
			hourAgo := time.Now().Add(-1 * time.Hour).Unix()
			if *child.LastActivityAt > hourAgo {
				activityIndicator = activityStyle.Render(" *")
			}
		}

		// Child count indicator
		childIndicator := ""
		if child.ChildCount > 0 {
			childIndicator = fmt.Sprintf(" +%d", child.ChildCount)
		}

		// Format: ├─ thread-name (5 msgs) *
		line := fmt.Sprintf("%s %s %s%s%s",
			treeStyle.Render(branch),
			m.zoneManager.Mark("subthread-"+child.GUID, nameStyle.Render(child.Name)),
			countStyle.Render(fmt.Sprintf("(%d msgs%s)", child.MessageCount, childIndicator)),
			activityIndicator,
			"",
		)
		lines = append(lines, line)
	}

	// If there are more children, show ellipsis
	totalChildren, _ := db.GetThreads(m.db, &types.ThreadQueryOptions{
		ParentThread: &m.currentThread.GUID,
	})
	if len(totalChildren) > maxChildren {
		more := len(totalChildren) - maxChildren
		lines = append(lines, treeStyle.Render(fmt.Sprintf("    ... +%d more", more)))
	}

	return strings.Join(lines, "\n")
}
