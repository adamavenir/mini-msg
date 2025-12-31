package command

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewNoteCmd creates the note command.
func NewNoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note <message>",
		Short: "Post a personal note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			if agentRef == "" {
				agentRef = os.Getenv("FRAY_AGENT_ID")
				if agentRef == "" {
					return writeCommandError(cmd, fmt.Errorf("--as is required or set FRAY_AGENT_ID"))
				}
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

			threadRef, _ := cmd.Flags().GetString("thread")
			var parent *types.Thread
			if threadRef != "" {
				parent, err = resolveThreadRef(ctx.DB, threadRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			threadName := fmt.Sprintf("%s-notes", agentID)
			if parent != nil {
				threadName = "notes"
			}

			thread, err := ensureThread(ctx, threadName, parent, []string{agentID})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			mentions := core.ExtractMentions(args[0], bases)
			mentions = core.ExpandAllMention(mentions, bases)

			now := time.Now().Unix()
			created, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: agentID,
				Body:      args[0],
				Mentions:  mentions,
				Home:      thread.GUID,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendMessage(ctx.Project.DBPath, created); err != nil {
				return writeCommandError(cmd, err)
			}

			updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread":  thread,
					"message": created,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Posted note to %s (%s)\n", thread.Name, thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to post as (defaults to FRAY_AGENT_ID)")
	cmd.Flags().String("thread", "", "parent thread for thread-local notes")

	return cmd
}

// NewNotesCmd creates the notes view command.
func NewNotesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notes",
		Short: "View your notes thread",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			if agentRef == "" {
				agentRef = os.Getenv("FRAY_AGENT_ID")
				if agentRef == "" {
					return writeCommandError(cmd, fmt.Errorf("--as is required or set FRAY_AGENT_ID"))
				}
			}
			agentID, err := resolveAgentRef(ctx, agentRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			threadName := fmt.Sprintf("%s-notes", agentID)
			thread, err := db.GetThreadByName(ctx.DB, threadName, nil)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if thread == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Notes thread not found")
				return nil
			}

			messages, err := db.GetThreadMessages(ctx.DB, thread.GUID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			messages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, messages)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread":   thread,
					"messages": messages,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Notes (%s)\n\n", thread.GUID)
			if len(messages) == 0 {
				fmt.Fprintln(out, "No notes yet")
				return nil
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			projectName := GetProjectName(ctx.Project.Root)
			for _, msg := range messages {
				fmt.Fprintln(out, FormatMessage(msg, projectName, bases))
			}
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to view notes for (defaults to FRAY_AGENT_ID)")

	return cmd
}
