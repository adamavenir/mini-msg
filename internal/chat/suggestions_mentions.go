package chat

import (
	"fmt"
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
)

func (m *Model) buildMentionSuggestions(prefix string) ([]suggestionItem, error) {
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}

	projectConfig, _ := db.ReadProjectConfig(m.projectDBPath)
	candidates := buildMentionCandidates(bases, projectConfig)

	normalized := strings.ToLower(prefix)
	suggestions := make([]suggestionItem, 0, suggestionLimit)
	for _, candidate := range candidates {
		matchingNicks := matchNickPrefix(candidate.Nicks, normalized)
		nameLower := strings.ToLower(candidate.Name)
		if normalized != "" && !strings.HasPrefix(nameLower, normalized) && len(matchingNicks) == 0 {
			continue
		}
		label := "@" + candidate.Name
		if len(matchingNicks) > 0 {
			label = fmt.Sprintf("@%s (aka %s)", candidate.Name, formatNickList(matchingNicks))
		}
		insert := "@" + candidate.Name
		suggestions = append(suggestions, suggestionItem{Display: label, Insert: insert})
		if len(suggestions) >= suggestionLimit {
			break
		}
	}
	return suggestions, nil
}

func buildMentionCandidates(bases map[string]struct{}, config *db.ProjectConfig) []mentionCandidate {
	nameToNicks := map[string][]string{}
	if config != nil && len(config.KnownAgents) > 0 {
		for _, entry := range config.KnownAgents {
			if entry.Name == nil || *entry.Name == "" {
				continue
			}
			name := core.NormalizeAgentRef(*entry.Name)
			if parsed, err := core.ParseAgentID(name); err == nil {
				name = parsed.Base
			}
			if name == "" {
				continue
			}
			nicks := normalizeNicks(entry.Nicks, name)
			if len(nicks) == 0 {
				continue
			}
			nameToNicks[name] = appendUnique(nameToNicks[name], nicks)
		}
	}

	candidates := make([]mentionCandidate, 0, len(bases)+1)
	for base := range bases {
		candidates = append(candidates, mentionCandidate{Name: base, Nicks: nameToNicks[base]})
	}
	if _, ok := bases["all"]; !ok {
		candidates = append(candidates, mentionCandidate{Name: "all"})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})
	return candidates
}

func normalizeNicks(nicks []string, name string) []string {
	if len(nicks) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		nick = core.NormalizeAgentRef(strings.TrimSpace(nick))
		if nick == "" || nick == name {
			continue
		}
		if _, ok := seen[nick]; ok {
			continue
		}
		seen[nick] = struct{}{}
		out = append(out, nick)
	}
	return out
}

func appendUnique(existing []string, added []string) []string {
	if len(added) == 0 {
		return existing
	}
	seen := map[string]struct{}{}
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for _, value := range added {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		existing = append(existing, value)
	}
	return existing
}

func matchNickPrefix(nicks []string, prefix string) []string {
	if prefix == "" || len(nicks) == 0 {
		return nil
	}
	out := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		if strings.HasPrefix(strings.ToLower(nick), prefix) {
			out = append(out, nick)
		}
	}
	return out
}

func formatNickList(nicks []string) string {
	formatted := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		if nick == "" {
			continue
		}
		formatted = append(formatted, "@"+nick)
	}
	return strings.Join(formatted, ", ")
}
