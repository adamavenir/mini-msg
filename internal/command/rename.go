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

// NewRenameCmd creates the rename command.
func NewRenameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename an agent",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			oldID, err := resolveAgentRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			newID := core.NormalizeAgentRef(args[1])

			if !core.IsValidAgentID(newID) {
				return writeCommandError(cmd, fmt.Errorf(
					"invalid agent name: %s\nNames must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens.\nExamples: alice, pm, eager-beaver, frontend-dev",
					newID,
				))
			}

			oldAgent, err := db.GetAgent(ctx.DB, oldID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if oldAgent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s", oldID))
			}

			staleHours := 4
			if value, err := db.GetConfig(ctx.DB, "stale_hours"); err == nil && value != "" {
				parsed := parseNumeric(value)
				if parsed > 0 {
					staleHours = parsed
				}
			}

			if existing, err := db.GetAgent(ctx.DB, newID); err != nil {
				return writeCommandError(cmd, err)
			} else if existing != nil {
				active, err := db.IsAgentActive(ctx.DB, newID, staleHours)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if active {
					return writeCommandError(cmd, fmt.Errorf("agent @%s already exists and is active.", newID))
				}
				return writeCommandError(cmd, fmt.Errorf(
					"agent @%s already exists (inactive).\nChoose a different name or wait for the inactive agent to be cleaned up.",
					newID,
				))
			}

			if err := db.RenameAgent(ctx.DB, oldID, newID); err != nil {
				return writeCommandError(cmd, err)
			}

			updated, err := db.GetAgent(ctx.DB, newID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if updated != nil {
				if err := db.AppendAgent(ctx.Project.DBPath, *updated); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			now := time.Now().Unix()
			posted, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: "system",
				Body:      fmt.Sprintf("@%s renamed to @%s", oldID, newID),
				Mentions:  []string{oldID, newID},
				Type:      types.MessageTypeAgent,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.AppendMessage(ctx.Project.DBPath, posted); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"old_id":  oldID,
					"new_id":  newID,
					"success": true,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Renamed @%s to @%s\n", oldID, newID)
			return nil
		},
	}

	return cmd
}
