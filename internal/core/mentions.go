package core

import (
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/adamavenir/fray/internal/types"
)

var (
	mentionRe            = regexp.MustCompile(`@([a-z][a-z0-9]*(?:[-\.][a-z0-9]+)*)`)
	mentionWithSessionRe = regexp.MustCompile(`@([a-z][a-z0-9]*(?:[-\.][a-z0-9]+)*)(?:#([a-zA-Z0-9]+))?`)
	// interruptMentionRe captures: (1) prefix !!, (2) agent name, (3) suffix !
	interruptMentionRe = regexp.MustCompile(`(!{1,2})@([a-z][a-z0-9]*(?:[-\.][a-z0-9]+)*)(!?)`)
	issueRefRe         = regexp.MustCompile(`@([a-z]+-[a-zA-Z0-9]+)`)
)

// MentionResult holds extracted mentions, fork sessions, and interrupts.
type MentionResult struct {
	Mentions     []string                      // Mention targets without @ prefix
	ForkSessions map[string]string             // Agent → session ID for @agent#sessid spawns
	Interrupts   map[string]types.InterruptInfo // Agent → interrupt info for !@agent patterns
}

// ExtractMentions returns mention targets without @ prefix.
func ExtractMentions(body string, agentBases map[string]struct{}) []string {
	matches := mentionRe.FindAllStringSubmatchIndex(body, -1)
	mentions := make([]string, 0, len(matches))

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start := match[0]
		if start > 0 {
			prev, _ := utf8.DecodeLastRuneInString(body[:start])
			if isAlphaNum(prev) {
				continue
			}
		}

		name := body[match[2]:match[3]]
		if name == "all" {
			mentions = append(mentions, name)
			continue
		}
		if containsDot(name) {
			mentions = append(mentions, name)
			continue
		}

		if agentBases != nil {
			if _, ok := agentBases[name]; ok {
				mentions = append(mentions, name)
			}
			continue
		}

		if len(name) >= 3 && len(name) <= 15 {
			mentions = append(mentions, name)
		}
	}

	return mentions
}

// ExtractMentionsWithSession extracts mentions with optional session IDs and interrupt syntax.
// Parses @agent#sessid syntax where sessid is optional.
// Parses !@agent, !!@agent, !@agent!, !!@agent! interrupt syntax.
// Returns MentionResult with mentions, fork_sessions, and interrupts maps.
func ExtractMentionsWithSession(body string, agentBases map[string]struct{}) MentionResult {
	matches := mentionWithSessionRe.FindAllStringSubmatchIndex(body, -1)
	result := MentionResult{
		Mentions:     make([]string, 0, len(matches)),
		ForkSessions: make(map[string]string),
		Interrupts:   make(map[string]types.InterruptInfo),
	}

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start := match[0]
		if start > 0 {
			prev, _ := utf8.DecodeLastRuneInString(body[:start])
			if isAlphaNum(prev) {
				continue
			}
		}

		name := body[match[2]:match[3]]

		// Extract session ID if present (groups 4-5)
		var sessionID string
		if len(match) >= 6 && match[4] != -1 && match[5] != -1 {
			sessionID = body[match[4]:match[5]]
		}

		if name == "all" {
			result.Mentions = append(result.Mentions, name)
			continue
		}
		if containsDot(name) {
			result.Mentions = append(result.Mentions, name)
			if sessionID != "" {
				result.ForkSessions[name] = sessionID
			}
			continue
		}

		if agentBases != nil {
			if _, ok := agentBases[name]; ok {
				result.Mentions = append(result.Mentions, name)
				if sessionID != "" {
					result.ForkSessions[name] = sessionID
				}
			}
			continue
		}

		if len(name) >= 3 && len(name) <= 15 {
			result.Mentions = append(result.Mentions, name)
			if sessionID != "" {
				result.ForkSessions[name] = sessionID
			}
		}
	}

	// Extract interrupt patterns (!@agent, !!@agent, !@agent!, !!@agent!)
	interruptMatches := interruptMentionRe.FindAllStringSubmatch(body, -1)
	for _, match := range interruptMatches {
		if len(match) < 4 {
			continue
		}
		prefix := match[1] // "!" or "!!"
		name := match[2]   // agent name
		suffix := match[3] // "" or "!"

		// Validate agent name
		isValid := false
		if name == "all" {
			isValid = true
		} else if containsDot(name) {
			isValid = true
		} else if agentBases != nil {
			_, isValid = agentBases[name]
		} else if len(name) >= 3 && len(name) <= 15 {
			isValid = true
		}

		if isValid {
			// Add to mentions if not already present
			found := false
			for _, m := range result.Mentions {
				if m == name {
					found = true
					break
				}
			}
			if !found {
				result.Mentions = append(result.Mentions, name)
			}

			// Record interrupt info
			result.Interrupts[name] = types.InterruptInfo{
				Double:  prefix == "!!",
				NoSpawn: suffix == "!",
			}
		}
	}

	return result
}

// ExtractIssueRefs finds @prefix-id style references.
func ExtractIssueRefs(body string) []string {
	matches := issueRefRe.FindAllStringSubmatch(body, -1)
	seen := map[string]struct{}{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		seen[lower(match[1])] = struct{}{}
	}

	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	return refs
}

// MatchesMention reports whether a mention matches an agent ID.
func MatchesMention(agentID, mentionPrefix string) bool {
	return MatchesPrefix(agentID, mentionPrefix)
}

// IsAllMention reports whether the mention is @all.
func IsAllMention(mention string) bool {
	return mention == "all"
}

// ExpandAllMention replaces "all" in mentions with all agent bases.
// This ensures @all messages appear in each agent's mention history.
func ExpandAllMention(mentions []string, agentBases map[string]struct{}) []string {
	hasAll := false
	for _, m := range mentions {
		if m == "all" {
			hasAll = true
			break
		}
	}
	if !hasAll {
		return mentions
	}

	seen := make(map[string]struct{})
	result := make([]string, 0, len(mentions)+len(agentBases))

	for _, m := range mentions {
		if m == "all" {
			for base := range agentBases {
				if _, ok := seen[base]; !ok {
					seen[base] = struct{}{}
					result = append(result, base)
				}
			}
		} else {
			if _, ok := seen[m]; !ok {
				seen[m] = struct{}{}
				result = append(result, m)
			}
		}
	}

	return result
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func containsDot(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] == '.' {
			return true
		}
	}
	return false
}

func lower(value string) string {
	buf := []rune(value)
	for i, r := range buf {
		buf[i] = unicode.ToLower(r)
	}
	return string(buf)
}
