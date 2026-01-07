package command

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
)

const maxDisplayLines = 20

// Accordion settings
const (
	DefaultAccordionThreshold = 10 // Show accordion if more than this many messages
	AccordionHeadCount        = 3  // Number of messages to show at start
	AccordionTailCount        = 3  // Number of messages to show at end
)

var (
	noColor = os.Getenv("NO_COLOR") != ""

	dim       = ansiCode("\x1b[2m")
	bold      = ansiCode("\x1b[1m")
	gray      = ansiCode("\x1b[38;5;240m")
	reset     = ansiCode("\x1b[0m")
	issueBG   = ansiCode("\x1b[48;5;17m")
	whiteText = ansiCode("\x1b[38;5;231m")
	cyan      = ansiCode("\x1b[36m")
)

var colorPairs = []struct {
	Bright string
	Dim    string
}{
	{Bright: ansiCode("\x1b[38;5;111m"), Dim: ansiCode("\x1b[38;5;105m")},
	{Bright: ansiCode("\x1b[38;5;157m"), Dim: ansiCode("\x1b[38;5;156m")},
	{Bright: ansiCode("\x1b[38;5;216m"), Dim: ansiCode("\x1b[38;5;215m")},
	{Bright: ansiCode("\x1b[38;5;36m"), Dim: ansiCode("\x1b[38;5;30m")},
	{Bright: ansiCode("\x1b[38;5;183m"), Dim: ansiCode("\x1b[38;5;141m")},
	{Bright: ansiCode("\x1b[38;5;230m"), Dim: ansiCode("\x1b[38;5;229m")},
}

var (
	mentionRe = regexp.MustCompile(`@([a-z][a-z0-9]*(?:[-\.][a-z0-9]+)*)`)
	issueRe   = regexp.MustCompile(`(?i)#([a-z][a-z0-9]*)-([a-z0-9]+)\b`)
)

// GetProjectName returns the basename for the project root.
func GetProjectName(projectRoot string) string {
	return filepath.Base(projectRoot)
}

// FormatMessage formats a message for display.
func FormatMessage(msg types.Message, projectName string, agentBases map[string]struct{}) string {
	return formatMessageWithOptions(msg, projectName, agentBases, true, nil)
}

// FormatMessageFull formats a message without truncation (for anchors).
func FormatMessageFull(msg types.Message, projectName string, agentBases map[string]struct{}) string {
	return formatMessageWithOptions(msg, projectName, agentBases, false, nil)
}

// FormatMessageWithQuote formats a message with an optional quoted message for inline display.
func FormatMessageWithQuote(msg types.Message, projectName string, agentBases map[string]struct{}, quotedMsg *types.Message) string {
	return formatMessageWithOptions(msg, projectName, agentBases, true, quotedMsg)
}

func formatMessageWithOptions(msg types.Message, projectName string, agentBases map[string]struct{}, truncate bool, quotedMsg *types.Message) string {
	editedSuffix := ""
	if msg.Edited || msg.EditCount > 0 || msg.EditedAt != nil {
		editedSuffix = " (edited)"
	}
	idBlock := fmt.Sprintf("%s[%s#%s%s %s]%s", dim, bold, projectName, reset, dim+msg.ID+editedSuffix, reset)

	// Check for answer message format
	if strings.HasPrefix(msg.Body, "answered @") {
		return formatAnswerMessage(msg, projectName, idBlock, agentBases)
	}

	color := getAgentColor(msg.FromAgent, msg.Type, nil)
	displayBody := msg.Body
	if truncate {
		displayBody = truncateForDisplay(msg.Body, msg.ID)
	}

	// Format quote block if present
	quoteBlock := ""
	if quotedMsg != nil {
		quoteBlock = formatQuoteBlock(quotedMsg, agentBases)
	}

	// Format reactions if present
	reactionSuffix := formatReactionSummary(msg.Reactions)
	if reactionSuffix != "" {
		reactionSuffix = " " + gray + "(" + reactionSuffix + ")" + reset
	}

	if color != "" {
		coloredBody := colorizeBody(displayBody, color, agentBases)
		coloredBody = highlightIssueIDs(coloredBody, color)
		if quoteBlock != "" {
			return fmt.Sprintf("%s %s@%s:%s\n%s\n\"%s\"%s%s", idBlock, color, msg.FromAgent, reset, quoteBlock, color+coloredBody, reset, reactionSuffix)
		}
		return fmt.Sprintf("%s %s@%s: \"%s\"%s%s", idBlock, color, msg.FromAgent, coloredBody, reset, reactionSuffix)
	}

	highlightedBody := highlightIssueIDs(highlightMentions(displayBody), "")
	if quoteBlock != "" {
		return fmt.Sprintf("%s @%s:\n%s\n\"%s\"%s", idBlock, msg.FromAgent, quoteBlock, highlightedBody, reactionSuffix)
	}
	return fmt.Sprintf("%s @%s: \"%s\"%s", idBlock, msg.FromAgent, highlightedBody, reactionSuffix)
}

// formatQuoteBlock formats the quoted message for inline display.
func formatQuoteBlock(quotedMsg *types.Message, _ map[string]struct{}) string {
	// Get first few lines of quoted message, truncated
	lines := strings.Split(quotedMsg.Body, "\n")

	// Take at most 3 lines, truncate each
	maxLines := 3
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	var quotedLines []string
	for _, line := range lines {
		if len(line) > 80 {
			line = line[:77] + "..."
		}
		quotedLines = append(quotedLines, gray+"> "+line+reset)
	}

	// Add source info
	shortID := quotedMsg.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	quotedLines = append(quotedLines, fmt.Sprintf("%s> [%s @%s]%s", gray, shortID, quotedMsg.FromAgent, reset))

	return strings.Join(quotedLines, "\n")
}

// formatAnswerMessage renders answer messages with Q&A colorization.
func formatAnswerMessage(msg types.Message, projectName, idBlock string, agentBases map[string]struct{}) string {
	lines := strings.Split(msg.Body, "\n")
	if len(lines) == 0 {
		return idBlock + " " + msg.Body
	}

	// Parse header: "answered @opus @designer"
	header := lines[0]
	askerPart := strings.TrimPrefix(header, "answered ")

	// Get colors
	answererColor := getAgentColor(msg.FromAgent, msg.Type, nil)
	if answererColor == "" {
		answererColor = reset
	}

	// Extract first asker for question color
	askerColor := gray
	if matches := mentionRe.FindStringSubmatch(askerPart); len(matches) > 1 {
		askerColor = getAgentColor(matches[1], types.MessageTypeAgent, nil)
		if askerColor == "" {
			askerColor = gray
		}
	}

	// Build formatted output
	var out strings.Builder

	// Header: @answerer answered @asker:
	out.WriteString(fmt.Sprintf("%s %s@%s%s answered %s:\n", idBlock, answererColor, msg.FromAgent, reset, askerPart))

	// Parse Q&A blocks
	inAnswer := false
	boldWhite := ansiCode("\x1b[1;37m")

	for i := 1; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "Q: ") {
			inAnswer = false
			questionText := strings.TrimPrefix(line, "Q: ")
			out.WriteString(fmt.Sprintf("%sQ:%s %s%s%s\n", boldWhite, reset, askerColor, questionText, reset))
		} else if strings.HasPrefix(line, "A: ") {
			inAnswer = true
			answerText := strings.TrimPrefix(line, "A: ")
			out.WriteString(fmt.Sprintf("%sA:%s %s%s%s\n", boldWhite, reset, answererColor, answerText, reset))
		} else if strings.TrimSpace(line) == "" {
			out.WriteString("\n")
		} else if inAnswer && strings.HasPrefix(line, "   ") {
			// Continuation of answer
			out.WriteString(fmt.Sprintf("%s%s%s\n", answererColor, line, reset))
		} else {
			out.WriteString(line + "\n")
		}
	}

	return strings.TrimRight(out.String(), "\n")
}

func ansiCode(code string) string {
	if noColor {
		return ""
	}
	return code
}

func getAgentColor(agentID string, msgType types.MessageType, colorMap map[string]int) string {
	if noColor {
		return ""
	}
	if msgType == types.MessageTypeUser {
		return ""
	}
	if agentID == "system" {
		return gray
	}

	parsed, err := core.ParseAgentID(agentID)
	if err != nil {
		index := hashString(agentID) % len(colorPairs)
		return colorPairs[index].Bright
	}

	base := parsed.Base
	colorIndex := hashString(base) % len(colorPairs)
	if colorMap != nil {
		if mapped, ok := colorMap[base]; ok {
			colorIndex = mapped % len(colorPairs)
		}
	}

	version := 1
	if parsed.Version != nil {
		version = *parsed.Version
	}

	pair := colorPairs[colorIndex]
	if version%2 == 1 {
		return pair.Bright
	}
	return pair.Dim
}

func hashString(value string) int {
	hash := 0
	for i := 0; i < len(value); i++ {
		hash = ((hash << 5) - hash) + int(value[i])
	}
	if hash < 0 {
		return -hash
	}
	return hash
}

func highlightIssueIDs(body, senderColor string) string {
	if noColor {
		return body
	}
	return issueRe.ReplaceAllStringFunc(body, func(match string) string {
		return issueBG + whiteText + match + reset + senderColor
	})
}

func highlightMentions(body string) string {
	if noColor {
		return body
	}
	matches := mentionRe.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return body
	}

	var out strings.Builder
	last := 0
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		start, end := match[0], match[1]
		if start > 0 {
			prev, _ := utf8.DecodeLastRuneInString(body[:start])
			if isAlphaNum(prev) {
				continue
			}
		}
		out.WriteString(body[last:start])
		out.WriteString(cyan)
		out.WriteString(body[start:end])
		out.WriteString(reset)
		last = end
	}
	out.WriteString(body[last:])
	return out.String()
}

func colorizeBody(body, senderColor string, agentBases map[string]struct{}) string {
	if noColor || senderColor == "" {
		return body
	}

	matches := mentionRe.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return body
	}

	var out strings.Builder
	last := 0
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start, end := match[0], match[1]
		nameStart, nameEnd := match[2], match[3]
		if start > 0 {
			prev, _ := utf8.DecodeLastRuneInString(body[:start])
			if isAlphaNum(prev) {
				continue
			}
		}

		out.WriteString(body[last:start])
		name := body[nameStart:nameEnd]
		replacement := body[start:end]
		lower := strings.ToLower(name)

		switch {
		case lower == "all":
			if mentionColor := getAgentColor(name, types.MessageTypeAgent, nil); mentionColor != "" {
				replacement = mentionColor + replacement + senderColor
			}
		case strings.Contains(name, "."):
			if mentionColor := getAgentColor(name, types.MessageTypeAgent, nil); mentionColor != "" {
				replacement = mentionColor + replacement + senderColor
			}
		case agentBases != nil:
			if _, ok := agentBases[lower]; ok {
				if mentionColor := getAgentColor(name, types.MessageTypeAgent, nil); mentionColor != "" {
					replacement = mentionColor + replacement + senderColor
				}
			} else if len(name) >= 3 && len(name) <= 15 && !strings.Contains(name, ".") {
				replacement = bold + replacement + reset + senderColor
			}
		default:
			if len(name) >= 3 && len(name) <= 15 {
				if mentionColor := getAgentColor(name, types.MessageTypeAgent, nil); mentionColor != "" {
					replacement = mentionColor + replacement + senderColor
				}
			}
		}

		out.WriteString(replacement)
		last = end
	}
	out.WriteString(body[last:])
	return out.String()
}

func truncateForDisplay(body, msgID string) string {
	lines := strings.Split(body, "\n")
	if len(lines) <= maxDisplayLines {
		return body
	}

	truncated := strings.Join(lines[:maxDisplayLines], "\n")
	remaining := len(lines) - maxDisplayLines
	return fmt.Sprintf("%s\n... (%d more lines. Use 'fray get %s' to see full)", truncated, remaining, msgID)
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// FormatMessagePreview formats a message as a one-line preview for accordion display.
// Format: "  [msg-abc123] @agent: First line of message..."
func FormatMessagePreview(msg types.Message, projectName string) string {
	// Get first line of body
	firstLine := strings.Split(msg.Body, "\n")[0]

	// Truncate if too long
	maxLen := 50
	if len(firstLine) > maxLen {
		firstLine = firstLine[:maxLen] + "..."
	}

	shortID := msg.ID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	return fmt.Sprintf("  %s[%s]%s @%s: %s", dim, shortID, reset, msg.FromAgent, firstLine)
}

// AccordionOptions configures accordion behavior.
type AccordionOptions struct {
	Threshold    int  // Show accordion if more than this many messages (0 = use default)
	HeadCount    int  // Messages to show at start (0 = use default)
	TailCount    int  // Messages to show at end (0 = use default)
	ShowAll      bool // If true, disable accordion and show all messages
	ProjectName  string
	AgentBases   map[string]struct{}
	QuotedMsgs   map[string]*types.Message // Map of message ID -> quoted message for inline display
}

// FormatMessageListAccordion formats a list of messages with accordion collapsing.
// Returns a slice of formatted lines ready to print.
func FormatMessageListAccordion(messages []types.Message, opts AccordionOptions) []string {
	if len(messages) == 0 {
		return nil
	}

	threshold := opts.Threshold
	if threshold == 0 {
		threshold = DefaultAccordionThreshold
	}
	headCount := opts.HeadCount
	if headCount == 0 {
		headCount = AccordionHeadCount
	}
	tailCount := opts.TailCount
	if tailCount == 0 {
		tailCount = AccordionTailCount
	}

	// Helper to format a message with its quote if available
	formatMsg := func(msg types.Message) string {
		var quotedMsg *types.Message
		if msg.QuoteMessageGUID != nil && opts.QuotedMsgs != nil {
			quotedMsg = opts.QuotedMsgs[*msg.QuoteMessageGUID]
		}
		return formatMessageWithOptions(msg, opts.ProjectName, opts.AgentBases, true, quotedMsg)
	}

	// If ShowAll or under threshold, format all messages normally
	if opts.ShowAll || len(messages) <= threshold {
		lines := make([]string, len(messages))
		for i, msg := range messages {
			lines[i] = formatMsg(msg)
		}
		return lines
	}

	// Accordion: head + collapsed middle + tail
	var lines []string

	// Head messages (full format)
	for i := 0; i < headCount && i < len(messages); i++ {
		lines = append(lines, formatMsg(messages[i]))
	}

	// Middle messages (preview format)
	middleStart := headCount
	middleEnd := len(messages) - tailCount
	if middleEnd > middleStart {
		collapsedCount := middleEnd - middleStart
		lines = append(lines, fmt.Sprintf("%s  ... %d messages collapsed ...%s", dim, collapsedCount, reset))
		for i := middleStart; i < middleEnd; i++ {
			lines = append(lines, FormatMessagePreview(messages[i], opts.ProjectName))
		}
		lines = append(lines, fmt.Sprintf("%s  ... end collapsed ...%s", dim, reset))
	}

	// Tail messages (full format)
	for i := middleEnd; i < len(messages); i++ {
		if i >= headCount { // Avoid duplicates if list is small
			lines = append(lines, formatMsg(messages[i]))
		}
	}

	return lines
}

// formatReactionSummary formats reactions for display.
// Single reaction: "👍 alice", multiple: "👍x3"
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
			parts = append(parts, fmt.Sprintf("%s %s", reaction, entries[0].AgentID))
		} else {
			parts = append(parts, fmt.Sprintf("%sx%d", reaction, count))
		}
	}
	return strings.Join(parts, " · ")
}
