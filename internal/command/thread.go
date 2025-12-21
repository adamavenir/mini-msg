package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewThreadCmd creates the thread command.
func NewThreadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thread <id>",
		Short: "View a message thread (parent + replies)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			messageID := strings.TrimPrefix(strings.TrimSpace(args[0]), "#")
			if messageID == "" {
				return writeCommandError(cmd, fmt.Errorf("message id is required"))
			}

			msg, err := db.GetMessage(ctx.DB, messageID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if msg == nil {
				return writeCommandError(cmd, fmt.Errorf("message %s not found", messageID))
			}

			thread, err := db.GetThread(ctx.DB, messageID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"parent_id": messageID,
					"messages":  renderThreadJSON(thread),
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			replyCount := len(thread) - 1
			label := "replies"
			if replyCount == 1 {
				label = "reply"
			}
			fmt.Fprintf(out, "Thread #%s (%d %s):\n\n", messageID, replyCount, label)

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			projectName := GetProjectName(ctx.Project.Root)
			for _, row := range thread {
				prefix := ""
				if row.ID != messageID {
					prefix = "  ↳ "
				}
				fmt.Fprintln(out, prefix+FormatMessage(row, projectName, bases))
			}

			return nil
		},
	}

	return cmd
}

func renderThreadJSON(messages []types.Message) []map[string]any {
	payload := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		payload = append(payload, map[string]any{
			"id":         msg.ID,
			"from_agent": msg.FromAgent,
			"body":       msg.Body,
			"reply_to":   msg.ReplyTo,
			"ts":         msg.TS,
		})
	}
	return payload
}
