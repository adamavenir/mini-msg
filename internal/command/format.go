package command

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
)

const maxDisplayLines = 20

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
	editedSuffix := ""
	if msg.Edited || msg.EditCount > 0 || msg.EditedAt != nil {
		editedSuffix = " (edited)"
	}
	idBlock := fmt.Sprintf("%s[%s#%s%s %s]%s", dim, bold, projectName, reset, dim+msg.ID+editedSuffix, reset)

	color := getAgentColor(msg.FromAgent, msg.Type, nil)
	truncated := truncateForDisplay(msg.Body, msg.ID)

	if color != "" {
		body := colorizeBody(truncated, color, agentBases)
		body = highlightIssueIDs(body, color)
		return fmt.Sprintf("%s %s@%s: \"%s\"%s", idBlock, color, msg.FromAgent, body, reset)
	}

	body := highlightIssueIDs(highlightMentions(truncated), "")
	return fmt.Sprintf("%s @%s: \"%s\"", idBlock, msg.FromAgent, body)
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
	return fmt.Sprintf("%s\n... (%d more lines. Use 'fray view %s' to see full)", truncated, remaining, msgID)
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
