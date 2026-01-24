package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewRmCmd creates the rm command.
func NewRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a message or thread",
		Long: `Delete a message or thread by ID.

The ID prefix determines the type:
  msg-xxxx   Delete a message
  thrd-xxxx  Delete (archive) a thread

Examples:
  fray rm msg-abc123
  fray rm thrd-xyz789
  fray rm design-thread --as opus`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			input := strings.TrimPrefix(strings.TrimSpace(args[0]), "#")

			// Check if it's a thread
			if strings.HasPrefix(input, "thrd-") {
				return deleteThread(cmd, ctx, input)
			}

			// Try to resolve as thread by name/path
			if !strings.HasPrefix(input, "msg-") {
				thread, err := resolveThreadRef(ctx.DB, input)
				if err == nil && thread != nil {
					return deleteThread(cmd, ctx, thread.GUID)
				}
			}

			// Otherwise treat as message
			return deleteMessage(cmd, ctx, input)
		},
	}

	cmd.Flags().String("as", "", "agent performing the deletion (for attribution)")

	return cmd
}

func deleteMessage(cmd *cobra.Command, ctx *CommandContext, input string) error {
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

	deletedBy, _ := cmd.Flags().GetString("as")
	var deletedByPtr *string
	if strings.TrimSpace(deletedBy) != "" {
		deletedByPtr = &deletedBy
	}
	deletedAt := int64(0)
	if updated.ArchivedAt != nil {
		deletedAt = *updated.ArchivedAt
	}
	if err := db.AppendMessageDelete(ctx.Project.DBPath, updated.ID, deletedByPtr, deletedAt); err != nil {
		return writeCommandError(cmd, err)
	}

	if ctx.JSONMode {
		payload := map[string]any{"id": updated.ID, "deleted": true}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deleted message %s\n", updated.ID)
	return nil
}

func deleteThread(cmd *cobra.Command, ctx *CommandContext, threadGUID string) error {
	thread, err := db.GetThread(ctx.DB, threadGUID)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	if thread == nil {
		return writeCommandError(cmd, fmt.Errorf("thread not found: %s", threadGUID))
	}

	// Archive the thread (soft delete)
	status := string(types.ThreadStatusArchived)
	updated, err := db.UpdateThread(ctx.DB, thread.GUID, db.ThreadUpdates{
		Status: types.OptionalString{Set: true, Value: &status},
	})
	if err != nil {
		return writeCommandError(cmd, err)
	}

	if err := db.AppendThreadDelete(ctx.Project.DBPath, updated.GUID, 0); err != nil {
		return writeCommandError(cmd, err)
	}

	if ctx.JSONMode {
		payload := map[string]any{"id": updated.GUID, "deleted": true, "type": "thread"}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}

	path, _ := buildThreadPath(ctx.DB, updated)
	if path == "" {
		path = updated.GUID
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted (archived) thread %s\n", path)
	return nil
}
