package command

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func resolveAgentRef(ctx *CommandContext, ref string) (string, error) {
	resolved := ResolveAgentRef(ref, ctx.ProjectConfig)
	suggestion, err := suggestAgentDelimiter(ctx.DB, resolved)
	if err != nil {
		return "", err
	}
	if suggestion != "" && !ctx.Force {
		return "", fmt.Errorf("did you mean @%s? Re-run with --force to use @%s", suggestion, resolved)
	}
	return resolved, nil
}

func resolveAgentByRef(ctx *CommandContext, ref string) (*types.Agent, error) {
	resolved, err := resolveAgentRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	agent, err := db.GetAgent(ctx.DB, resolved)
	if err != nil {
		return nil, err
	}
	if agent != nil {
		return agent, nil
	}

	matches, err := db.GetAgentsByPrefix(ctx.DB, resolved)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("agent not found: %s", ref)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.AgentID)
		}
		return nil, fmt.Errorf("ambiguous prefix '%s' matches: %s", ref, joinList(ids))
	}
	return &matches[0], nil
}

func suggestAgentDelimiter(dbConn *sql.DB, agentRef string) (string, error) {
	base := baseFromAgentRef(agentRef)
	if base == "" {
		return "", nil
	}
	bases, err := db.GetAgentBases(dbConn)
	if err != nil {
		return "", err
	}
	if _, ok := bases[base]; ok {
		return "", nil
	}

	key := delimiterKey(base)
	if key == "" {
		return "", nil
	}
	candidates := make([]string, 0)
	for candidate := range bases {
		if candidate == base {
			continue
		}
		if delimiterKey(candidate) == key {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return "", nil
	}
	sort.Strings(candidates)
	return candidates[0], nil
}

func baseFromAgentRef(ref string) string {
	normalized := core.NormalizeAgentRef(strings.TrimSpace(ref))
	if core.IsLegacyAgentID(normalized) {
		lastDot := strings.LastIndex(normalized, ".")
		if lastDot > 0 {
			return normalized[:lastDot]
		}
	}
	return normalized
}

func delimiterKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", "", ".", "")
	return replacer.Replace(value)
}

func joinList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for i := 1; i < len(values); i++ {
		out += ", " + values[i]
	}
	return out
}
