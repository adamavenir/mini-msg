package command

import (
	"fmt"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
)

func resolveAgentByRef(ctx *CommandContext, ref string) (*types.Agent, error) {
	resolved := ResolveAgentRef(ref, ctx.ProjectConfig)

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
