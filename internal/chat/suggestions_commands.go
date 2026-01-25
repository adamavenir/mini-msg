package chat

import (
	"fmt"
	"strings"
)

// getCommandUsage returns the usage string for a completed command.
func getCommandUsage(cmdName string) string {
	cmdName = strings.ToLower(cmdName)
	for _, cmd := range allCommands {
		if strings.ToLower(cmd.Name) == cmdName && cmd.Usage != "" {
			return fmt.Sprintf("%s %s", cmd.Name, cmd.Usage)
		}
	}
	return ""
}

// buildCommandSuggestions returns command suggestions that match the prefix.
// Prioritizes exact prefix matches, then fuzzy matches.
func buildCommandSuggestions(prefix string) []suggestionItem {
	prefix = strings.ToLower(prefix)
	var prefixMatches []suggestionItem
	var fuzzyMatches []suggestionItem

	for _, cmd := range allCommands {
		// Command name without the leading slash for matching
		cmdName := strings.ToLower(cmd.Name[1:])
		display := fmt.Sprintf("%s  %s", cmd.Name, cmd.Desc)
		item := suggestionItem{Display: display, Insert: cmd.Name}

		if strings.HasPrefix(cmdName, prefix) {
			prefixMatches = append(prefixMatches, item)
		} else if fuzzyMatch(cmdName, prefix) {
			fuzzyMatches = append(fuzzyMatches, item)
		}
	}

	// Combine: prefix matches first, then fuzzy matches
	suggestions := append(prefixMatches, fuzzyMatches...)
	if len(suggestions) > suggestionLimit {
		suggestions = suggestions[:suggestionLimit]
	}
	return suggestions
}

// fuzzyMatch checks if needle characters appear in order within haystack.
// Empty needle matches everything.
func fuzzyMatch(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	hi := 0
	for _, ch := range needle {
		found := false
		for hi < len(haystack) {
			if rune(haystack[hi]) == ch {
				hi++
				found = true
				break
			}
			hi++
		}
		if !found {
			return false
		}
	}
	return true
}
