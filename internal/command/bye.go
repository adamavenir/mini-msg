package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewByeCmd creates the bye command.
func NewByeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bye <agent> [message]",
		Short: "Leave agent session",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID, err := resolveAgentRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			message := ""
			if len(args) > 1 {
				message = args[1]
			}

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", agentID))
			}

			now := time.Now().Unix()
			clearedClaims, err := db.DeleteClaimsByAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			var posted *types.Message
			if message != "" {
				bases, err := db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				mentions := core.ExtractMentions(message, bases)
				mentions = core.ExpandAllMention(mentions, bases)
				created, err := db.CreateMessage(ctx.DB, types.Message{
					TS:        now,
					FromAgent: agentID,
					Body:      message,
					Mentions:  mentions,
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendMessage(ctx.Project.DBPath, created); err != nil {
					return writeCommandError(cmd, err)
				}
				posted = &created
			}

			updates := db.AgentUpdates{
				LeftAt:   types.OptionalInt64{Set: true, Value: &now},
				LastSeen: types.OptionalInt64{Set: true, Value: &now},
			}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}
			updated, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if updated != nil {
				if err := db.AppendAgent(ctx.Project.DBPath, *updated); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"agent_id":       agentID,
					"status":         "left",
					"message_id":     nil,
					"claims_cleared": clearedClaims,
				}
				if posted != nil {
					payload["message_id"] = posted.ID
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Goodbye, @%s!\n", agentID)
			if posted != nil {
				fmt.Fprintf(out, "  Posted: [%s] %s\n", posted.ID, message)
			}
			if clearedClaims > 0 {
				plural := "s"
				if clearedClaims == 1 {
					plural = ""
				}
				fmt.Fprintf(out, "  Released %d claim%s\n", clearedClaims, plural)
			}
			return nil
		},
	}

	return cmd
}
