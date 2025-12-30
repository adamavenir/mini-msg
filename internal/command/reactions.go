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
		users := msg.Reactions[reaction]
		if len(users) == 0 {
			continue
		}
		lines = append(lines, core.FormatReactionEvent(users, reaction, msg.ID, msg.Body))
	}
	return lines
}
