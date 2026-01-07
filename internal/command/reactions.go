package command

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
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

// NewReactionsCmd creates the reactions query command.
func NewReactionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reactions",
		Short: "List reaction activity",
		Long: `Show reaction activity across threads.

Examples:
  fray reactions --by alice   Messages alice has reacted to
  fray reactions --to alice   Reactions on alice's messages`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			byAgent, _ := cmd.Flags().GetString("by")
			toAgent, _ := cmd.Flags().GetString("to")
			limit, _ := cmd.Flags().GetInt("last")

			if byAgent == "" && toAgent == "" {
				return writeCommandError(cmd, fmt.Errorf("one of --by or --to is required"))
			}
			if byAgent != "" && toAgent != "" {
				return writeCommandError(cmd, fmt.Errorf("use --by or --to, not both"))
			}

			if limit == 0 {
				limit = 20
			}

			out := cmd.OutOrStdout()

			if byAgent != "" {
				agentID := ResolveAgentRef(byAgent, ctx.ProjectConfig)
				results, err := db.GetMessagesReactedToByAgent(ctx.DB, agentID, limit)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				if ctx.JSONMode {
					return json.NewEncoder(out).Encode(results)
				}

				if len(results) == 0 {
					fmt.Fprintf(out, "@%s hasn't reacted to any messages\n", agentID)
					return nil
				}

				fmt.Fprintf(out, "Messages @%s reacted to:\n\n", agentID)
				for _, r := range results {
					preview := truncateBody(r.Body, 60)
					fmt.Fprintf(out, "  %s %s on %s (by @%s): %s\n",
						r.Emoji, r.MessageGUID, formatHome(r.Home), r.FromAgent, preview)
				}
				return nil
			}

			if toAgent != "" {
				agentID := ResolveAgentRef(toAgent, ctx.ProjectConfig)
				results, err := db.GetMessagesWithReactionsFrom(ctx.DB, agentID, limit)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				if ctx.JSONMode {
					return json.NewEncoder(out).Encode(results)
				}

				if len(results) == 0 {
					fmt.Fprintf(out, "No reactions on @%s's messages\n", agentID)
					return nil
				}

				fmt.Fprintf(out, "Reactions on @%s's messages:\n\n", agentID)
				for _, r := range results {
					preview := truncateBody(r.Body, 60)
					fmt.Fprintf(out, "  %s by @%s on %s: %s\n",
						r.Emoji, r.ReactedBy, r.MessageGUID, preview)
				}
				return nil
			}

			return nil
		},
	}

	cmd.Flags().String("by", "", "show messages an agent has reacted to")
	cmd.Flags().String("to", "", "show reactions on an agent's messages")
	cmd.Flags().Int("last", 20, "limit results")

	return cmd
}

func truncateBody(body string, maxLen int) string {
	body = strings.TrimSpace(body)
	if idx := strings.Index(body, "\n"); idx > 0 && idx < maxLen {
		body = body[:idx]
	}
	if len(body) > maxLen {
		return body[:maxLen-3] + "..."
	}
	return body
}

func formatHome(home string) string {
	if home == "room" || home == "" {
		return "room"
	}
	return home
}
