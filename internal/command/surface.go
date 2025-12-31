package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewSurfaceCmd creates the surface command.
func NewSurfaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "surface <message> <comment>",
		Short: "Surface a message to the room with a backlink",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			if agentRef == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required"))
			}
			agentID, err := resolveAgentRef(ctx, agentRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s. Use 'fray new' first", agentID))
			}
			if agent.LeftAt != nil {
				return writeCommandError(cmd, fmt.Errorf("agent @%s has left. Use 'fray back @%s' to resume", agentID, agentID))
			}

			original, err := resolveMessageRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			mentions := core.ExtractMentions(args[1], bases)
			mentions = core.ExpandAllMention(mentions, bases)

			now := time.Now().Unix()
			reference := original.ID
			surfaceMessage, err := db.CreateMessage(ctx.DB, types.Message{
				TS:         now,
				FromAgent:  agentID,
				Body:       args[1],
				Mentions:   mentions,
				Type:       types.MessageTypeSurface,
				References: &reference,
				Home:       "room",
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendMessage(ctx.Project.DBPath, surfaceMessage); err != nil {
				return writeCommandError(cmd, err)
			}

			updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			if original.Home != "" && original.Home != "room" {
				surfaceID := surfaceMessage.ID
				backlink := fmt.Sprintf("surface: @%s surfaced #%s", agentID, surfaceID)
				eventMessage, err := db.CreateMessage(ctx.DB, types.Message{
					TS:             now,
					FromAgent:      "system",
					Body:           backlink,
					Type:           types.MessageTypeEvent,
					References:     &reference,
					SurfaceMessage: &surfaceID,
					Home:           original.Home,
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendMessage(ctx.Project.DBPath, eventMessage); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"surface": surfaceMessage,
					"source":  original.ID,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Surfaced message %s as %s\n", original.ID, surfaceMessage.ID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to surface as")
	_ = cmd.MarkFlagRequired("as")

	return cmd
}
