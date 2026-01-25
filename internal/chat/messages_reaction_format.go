package chat

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

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
	pillBg := lipgloss.Color("236") // dim grey bg
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
