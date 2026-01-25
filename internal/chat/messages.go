package chat

import (
	"fmt"
	"regexp"
	"strings"

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
//
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
