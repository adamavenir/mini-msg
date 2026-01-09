package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/command/hooks"
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewBackCmd creates the back command.
func NewBackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "back <agent> [message]",
		Short: "Rejoin as a previous agent",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			jsonMode, _ := cmd.Flags().GetBool("json")

			agentID, err := resolveAgentRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", agentID))
			}

			// Check if there was a prior bye (left_at was set)
			hadPriorBye := agent.LeftAt != nil

			// Clear ghost cursor session acks so cursors become "unread" for new session
			if err := db.ClearGhostCursorSessionAcks(ctx.DB, agentID); err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			updates := db.AgentUpdates{
				LastSeen: types.OptionalInt64{Set: true, Value: &now},
				LeftAt:   types.OptionalInt64{Set: true, Value: nil},
			}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			// For managed agents, set presence to active so daemon doesn't spawn duplicates
			if agent.Managed {
				if err := db.UpdateAgentPresence(ctx.DB, agentID, types.PresenceActive); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if updated, err := db.GetAgent(ctx.DB, agentID); err == nil && updated != nil {
				if err := db.AppendAgent(ctx.Project.DBPath, *updated); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			// Ensure agent thread hierarchy exists (backfills missing threads for existing agents)
			if err := ensureAgentHierarchy(ctx, agentID); err != nil {
				return writeCommandError(cmd, err)
			}

			// Ensure agent neo file exists (backfills for existing agents)
			if err := ensureAgentNeo(ctx.Project.Root, agentID); err != nil {
				return writeCommandError(cmd, err)
			}

			// Post event message for the session change
			eventBody := fmt.Sprintf("@%s rejoined", agentID)
			if !hadPriorBye {
				eventBody = fmt.Sprintf("new @%s session", agentID)
			}
			eventMsg, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: agentID,
				Body:      eventBody,
				Type:      types.MessageTypeEvent,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.AppendMessage(ctx.Project.DBPath, eventMsg); err != nil {
				return writeCommandError(cmd, err)
			}

			var postedID *string
			if len(args) > 1 {
				message := strings.TrimSpace(args[1])
				if message != "" {
					bases, err := db.GetAgentBases(ctx.DB)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					mentions := core.ExtractMentions(message, bases)
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
					postedID = &created.ID
				}
			}

			wroteEnv := hooks.WriteClaudeEnv(agentID)

			if jsonMode {
				payload := map[string]any{
					"agent_id":   agentID,
					"status":     "active",
					"message_id": postedID,
					"claude_env": wroteEnv,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Welcome back, @%s!\n", agentID)
			if agent.Status != nil && *agent.Status != "" {
				fmt.Fprintf(out, "  Status: %s\n", *agent.Status)
			}
			if postedID != nil && len(args) > 1 {
				fmt.Fprintf(out, "  Posted: [%s] %s\n", *postedID, args[1])
			}
			if wroteEnv {
				fmt.Fprintln(out, "  Registered with Claude hooks")
			}
			return nil
		},
	}

	return cmd
}
