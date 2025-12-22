package command

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewUnreactCmd creates the unreact command.
func NewUnreactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unreact <msgid>",
		Short: "Remove your reactions from a message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			if agentRef == "" {
				agentRef = os.Getenv("MM_AGENT_ID")
			}
			if agentRef == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required or set MM_AGENT_ID"))
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
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", agentID))
			}

			input := strings.TrimPrefix(strings.TrimSpace(args[0]), "#")
			msg, err := db.GetMessage(ctx.DB, input)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if msg == nil {
				msg, err = db.GetMessage(ctx.DB, "msg-"+input)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}
			if msg == nil {
				msg, err = db.GetMessageByPrefix(ctx.DB, input)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}
			if msg == nil {
				return writeCommandError(cmd, fmt.Errorf("message not found: %s", input))
			}

			updated, changed, err := db.RemoveReactions(ctx.DB, msg.ID, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			if changed {
				update := db.MessageUpdateJSONLRecord{ID: updated.ID, Reactions: &updated.Reactions}
				if err := db.AppendMessageUpdate(ctx.Project.DBPath, update); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"id":      msg.ID,
					"agent":   agentID,
					"removed": changed,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if changed {
				fmt.Fprintf(out, "Removed reactions from #%s\n", msg.ID)
				return nil
			}
			fmt.Fprintf(out, "No reactions to remove for @%s on #%s\n", agentID, msg.ID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID removing the reaction (defaults to MM_AGENT_ID)")
	return cmd
}
