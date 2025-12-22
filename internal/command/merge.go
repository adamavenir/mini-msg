package command

import (
	"encoding/json"
	"fmt"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewMergeCmd creates the merge command.
func NewMergeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge <from> <to>",
		Short: "Merge message history from one agent into another",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			fromID, err := resolveAgentRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			toID, err := resolveAgentRef(ctx, args[1])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if fromID == toID {
				return writeCommandError(cmd, fmt.Errorf("cannot merge @%s into itself", fromID))
			}

			source, err := db.GetAgent(ctx.DB, fromID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if source == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", fromID))
			}

			target, err := db.GetAgent(ctx.DB, toID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if target == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", toID))
			}

			moved, err := db.MergeAgentHistory(ctx.DB, fromID, toID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if source.LastSeen > target.LastSeen {
				value := source.LastSeen
				if err := db.UpdateAgent(ctx.DB, toID, db.AgentUpdates{
					LastSeen: types.OptionalInt64{Set: true, Value: &value},
				}); err != nil {
					return writeCommandError(cmd, err)
				}
				updated, err := db.GetAgent(ctx.DB, toID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if updated != nil {
					if err := db.AppendAgent(ctx.Project.DBPath, *updated); err != nil {
						return writeCommandError(cmd, err)
					}
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"from":           fromID,
					"to":             toID,
					"messages_moved": moved,
					"merged":         true,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if moved > 0 {
				fmt.Fprintf(out, "Merged @%s into @%s (moved %d messages)\n", fromID, toID, moved)
				return nil
			}
			fmt.Fprintf(out, "Merged @%s into @%s\n", fromID, toID)
			return nil
		},
	}

	return cmd
}
