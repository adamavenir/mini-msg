package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// NewRmCmd creates the rm command.
func NewRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <msgid>",
		Short: "Delete a message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

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

			if err := db.DeleteMessage(ctx.DB, msg.ID); err != nil {
				return writeCommandError(cmd, err)
			}

			updated, err := db.GetMessage(ctx.DB, msg.ID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if updated == nil {
				return writeCommandError(cmd, fmt.Errorf("message %s not found", msg.ID))
			}

			update := db.MessageUpdateJSONLRecord{ID: updated.ID, ArchivedAt: updated.ArchivedAt}
			body := updated.Body
			update.Body = &body
			if err := db.AppendMessageUpdate(ctx.Project.DBPath, update); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{"id": updated.ID, "deleted": true}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Deleted message #%s\n", updated.ID)
			return nil
		},
	}

	return cmd
}
