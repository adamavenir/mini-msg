package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/spf13/cobra"
)

// NewEditCmd creates the edit command.
func NewEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <msgid> <message>",
		Short: "Edit a message you posted",
		Args:  cobra.MinimumNArgs(2),
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
			agentID := ResolveAgentRef(agentRef, ctx.ProjectConfig)

			msgID := strings.TrimPrefix(strings.TrimSpace(args[0]), "#")
			newBody := strings.Join(args[1:], " ")

			if err := db.EditMessage(ctx.DB, msgID, newBody, agentID); err != nil {
				return writeCommandError(cmd, err)
			}

			updated, err := db.GetMessage(ctx.DB, msgID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if updated == nil {
				return writeCommandError(cmd, fmt.Errorf("message %s not found", msgID))
			}

			update := db.MessageUpdateJSONLRecord{ID: updated.ID, EditedAt: updated.EditedAt}
			body := updated.Body
			update.Body = &body
			if err := db.AppendMessageUpdate(ctx.Project.DBPath, update); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{"id": updated.ID, "edited": true}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Edited message #%s\n", updated.ID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID editing the message")
	_ = cmd.MarkFlagRequired("as")

	return cmd
}
