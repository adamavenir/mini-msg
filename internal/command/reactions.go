package command

import (
	"sort"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
)

func formatReactionEvents(msg types.Message) []string {
	if len(msg.Reactions) == 0 {
		return nil
	}
	reactions := make([]string, 0, len(msg.Reactions))
	for reaction := range msg.Reactions {
		reactions = append(reactions, reaction)
	}
	sort.Strings(reactions)

	lines := make([]string, 0, len(reactions))
	for _, reaction := range reactions {
		entries := msg.Reactions[reaction]
		if len(entries) == 0 {
			continue
		}
		// Extract agent IDs from entries
		agents := make([]string, 0, len(entries))
		for _, e := range entries {
			agents = append(agents, e.AgentID)
		}
		lines = append(lines, core.FormatReactionEvent(agents, reaction, msg.ID, msg.Body))
	}
	return lines
}
