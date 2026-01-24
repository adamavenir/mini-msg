package command

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewCursorCmd creates the cursor command group.
func NewCursorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cursor",
		Short: "Manage ghost cursors for session handoffs",
		Long: `Ghost cursors mark where incoming agents should start reading from.

Unlike real read position (where you actually read to), ghost cursor is where the
outgoing agent says the next agent should START reading from to get context.

Use case: Agent leaves at msg-100, but the relevant context starts at msg-50.
Ghost cursor = msg-50. Next agent gets msg-50→current.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newCursorSetCmd(),
		newCursorStageCmd(),
		newCursorShowCmd(),
		newCursorClearCmd(),
	)

	return cmd
}

func newCursorSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <agent> <home> <message>",
		Short: "Set a ghost cursor for session handoff",
		Long: `Set a ghost cursor to mark where the next agent should start reading.

Arguments:
  agent    Agent ID (e.g., opus, @opus)
  home     "room" or thread GUID/name
  message  Message GUID to start reading from

Use --must-read to mark that the content should be injected fully in neo,
rather than just showing a hint like "3 new in design-thread".`,
		Args: cobra.ExactArgs(3),
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

			home := args[1]
			if home != "room" {
				// Try to resolve as thread
				thread, err := resolveThreadRef(ctx.DB, home)
				if err != nil {
					return writeCommandError(cmd, fmt.Errorf("invalid home: %s (must be 'room' or thread reference)", home))
				}
				home = thread.GUID
			}

			message, err := resolveMessageRef(ctx.DB, args[2])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			messageGUID := message.ID

			mustRead, _ := cmd.Flags().GetBool("must-read")

			cursor := types.GhostCursor{
				AgentID:     agentID,
				Home:        home,
				MessageGUID: messageGUID,
				MustRead:    mustRead,
				SetAt:       time.Now().UnixMilli(),
			}

			if err := db.SetGhostCursor(ctx.DB, cursor); err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendGhostCursor(ctx.Project.DBPath, cursor); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id":     agentID,
					"home":         home,
					"message_guid": messageGUID,
					"must_read":    mustRead,
				})
			}

			out := cmd.OutOrStdout()
			mustReadStr := ""
			if mustRead {
				mustReadStr = " (must-read)"
			}
			fmt.Fprintf(out, "Ghost cursor set for @%s in %s: %s%s\n", agentID, home, messageGUID, mustReadStr)
			return nil
		},
	}

	cmd.Flags().Bool("must-read", false, "inject full content vs hint only")

	return cmd
}

func newCursorStageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stage <home> <message>",
		Short: "Stage a ghost cursor for commit on bye/brb",
		Long: `Stage a ghost cursor to be committed when agent exits.

Staged cursors are temporary and only become official ghost cursors when:
- Agent runs 'fray bye' or 'fray brb'
- All staged cursors are committed at exit time

This allows agents to set multiple handoff points during a session without
committing them immediately. Uses FRAY_AGENT_ID for agent identity.

Arguments:
  home     "room" or thread GUID/name
  message  Message GUID to start reading from

Use --must-read to mark that content should be injected fully in neo.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := os.Getenv("FRAY_AGENT_ID")
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("FRAY_AGENT_ID not set (required for staging cursors)"))
			}

			home := args[0]
			if home != "room" {
				// Try to resolve as thread
				thread, err := resolveThreadRef(ctx.DB, home)
				if err != nil {
					return writeCommandError(cmd, fmt.Errorf("invalid home: %s (must be 'room' or thread reference)", home))
				}
				home = thread.GUID
			}

			message, err := resolveMessageRef(ctx.DB, args[1])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			messageGUID := message.ID

			mustRead, _ := cmd.Flags().GetBool("must-read")

			cursor := types.GhostCursor{
				AgentID:     agentID,
				Home:        home,
				MessageGUID: messageGUID,
				MustRead:    mustRead,
				SetAt:       time.Now().UnixMilli(),
			}

			if err := db.SetStagedCursor(ctx.DB, cursor); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id":     agentID,
					"home":         home,
					"message_guid": messageGUID,
					"must_read":    mustRead,
					"staged":       true,
				})
			}

			out := cmd.OutOrStdout()
			mustReadStr := ""
			if mustRead {
				mustReadStr = " (must-read)"
			}
			fmt.Fprintf(out, "Staged cursor for @%s in %s: %s%s\n", agentID, home, messageGUID, mustReadStr)
			fmt.Fprintln(out, "Will be committed on 'fray bye' or 'fray brb'")
			return nil
		},
	}

	cmd.Flags().Bool("must-read", false, "inject full content vs hint only")

	return cmd
}

func newCursorShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <agent>",
		Short: "Show ghost cursors for an agent",
		Args:  cobra.ExactArgs(1),
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

			cursors, err := db.GetGhostCursors(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id": agentID,
					"cursors":  cursors,
				})
			}

			out := cmd.OutOrStdout()
			if len(cursors) == 0 {
				fmt.Fprintf(out, "No ghost cursors for @%s\n", agentID)
				return nil
			}

			fmt.Fprintf(out, "Ghost cursors for @%s:\n", agentID)
			for _, cursor := range cursors {
				mustReadStr := ""
				if cursor.MustRead {
					mustReadStr = " [must-read]"
				}
				homeName := cursor.Home
				if cursor.Home != "room" {
					if thread, err := db.GetThread(ctx.DB, cursor.Home); err == nil && thread != nil {
						homeName = thread.Name
					}
				}
				fmt.Fprintf(out, "  %s: %s%s\n", homeName, cursor.MessageGUID, mustReadStr)
			}
			return nil
		},
	}

	return cmd
}

func newCursorClearCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clear <agent> [home]",
		Short: "Clear ghost cursors for an agent",
		Long: `Clear ghost cursors for an agent.

If home is specified, only clear that cursor. Otherwise clear all cursors.`,
		Args: cobra.RangeArgs(1, 2),
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

			if len(args) > 1 {
				home := args[1]
				if home != "room" {
					thread, err := resolveThreadRef(ctx.DB, home)
					if err != nil {
						return writeCommandError(cmd, fmt.Errorf("invalid home: %s", home))
					}
					home = thread.GUID
				}

				if err := db.DeleteGhostCursor(ctx.DB, agentID, home); err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendCursorClear(ctx.Project.DBPath, agentID, home, time.Now().UnixMilli()); err != nil {
					return writeCommandError(cmd, err)
				}

				// Append a "clear" event by setting an empty cursor
				// Actually, we just don't append anything - the JSONL will just not have a cursor for this home
				// The rebuild will reflect the current state

				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"agent_id": agentID,
						"home":     home,
						"cleared":  true,
					})
				}

				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "Cleared ghost cursor for @%s in %s\n", agentID, home)
				return nil
			}

			cursors, err := db.GetGhostCursors(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.DeleteAllGhostCursors(ctx.DB, agentID); err != nil {
				return writeCommandError(cmd, err)
			}
			if len(cursors) > 0 {
				clearedAt := time.Now().UnixMilli()
				for _, cursor := range cursors {
					if err := db.AppendCursorClear(ctx.Project.DBPath, agentID, cursor.Home, clearedAt); err != nil {
						return writeCommandError(cmd, err)
					}
				}
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"agent_id":    agentID,
					"cleared_all": true,
				})
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Cleared all ghost cursors for @%s\n", agentID)
			return nil
		},
	}

	return cmd
}
