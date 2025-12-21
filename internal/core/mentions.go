package core

import (
	"regexp"
	"unicode"
	"unicode/utf8"
)

var (
	mentionRe  = regexp.MustCompile(`@([a-z][a-z0-9]*(?:[-\.][a-z0-9]+)*)`)
	issueRefRe = regexp.MustCompile(`@([a-z]+-[a-zA-Z0-9]+)`)
)

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
