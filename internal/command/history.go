package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewHistoryCmd creates the history command.
func NewHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history <agent>",
		Short: "Show message history for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agent, err := resolveAgentByRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			sinceValue, beforeValue, err := parseTimeRangeFlags(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			var sinceCursor *types.MessageCursor
			if sinceValue != "" {
				cursor, err := core.ParseTimeExpression(ctx.DB, sinceValue, "since")
				if err != nil {
					return writeCommandError(cmd, err)
				}
				sinceCursor = cursor
			}
			var beforeCursor *types.MessageCursor
			if beforeValue != "" {
				cursor, err := core.ParseTimeExpression(ctx.DB, beforeValue, "before")
				if err != nil {
					return writeCommandError(cmd, err)
				}
				beforeCursor = cursor
			}

			messages, err := db.GetMessages(ctx.DB, &types.MessageQueryOptions{Since: sinceCursor, Before: beforeCursor})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			rows := make([]types.Message, 0)
			for _, msg := range messages {
				if msg.FromAgent == agent.AgentID {
					rows = append(rows, msg)
				}
			}
			rows, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, rows)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := buildHistoryPayload(agent, rows)
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if len(rows) == 0 {
				fmt.Fprintf(out, "No messages from @%s\n", agent.AgentID)
				return nil
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			projectName := GetProjectName(ctx.Project.Root)
			for _, row := range rows {
				fmt.Fprintln(out, FormatMessage(row, projectName, bases))
			}
			return nil
		},
	}

	cmd.Flags().String("since", "", "show messages after time or GUID")
	cmd.Flags().String("before", "", "show messages before time or GUID")
	cmd.Flags().String("from", "", "range start (time or GUID)")
	cmd.Flags().String("to", "", "range end (time or GUID)")

	return cmd
}

func buildHistoryPayload(agent *types.Agent, rows []types.Message) map[string]any {
	now := time.Now().Unix()
	messages := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		created := time.Unix(row.TS, 0).UTC().Format(time.RFC3339)
		messages = append(messages, map[string]any{
			"id":          row.ID,
			"agent_id":    agent.GUID,
			"body":        row.Body,
			"created_at":  created,
			"age_seconds": maxInt64(0, now-row.TS),
			"mentions":    row.Mentions,
			"reply_to":    row.ReplyTo,
			"edited":      row.Edited,
			"edit_count":  row.EditCount,
		})
	}
	return map[string]any{
		"agent":    agent.AgentID,
		"agent_id": agent.GUID,
		"messages": messages,
		"total":    len(messages),
	}
}

func parseTimeRangeFlags(cmd *cobra.Command) (string, string, error) {
	sinceValue, _ := cmd.Flags().GetString("since")
	beforeValue, _ := cmd.Flags().GetString("before")
	fromValue, _ := cmd.Flags().GetString("from")
	toValue, _ := cmd.Flags().GetString("to")

	if sinceValue != "" && fromValue != "" {
		return "", "", fmt.Errorf("use --since or --from, not both")
	}
	if beforeValue != "" && toValue != "" {
		return "", "", fmt.Errorf("use --before or --to, not both")
	}
	if sinceValue == "" {
		sinceValue = fromValue
	}
	if beforeValue == "" {
		beforeValue = toValue
	}
	return sinceValue, beforeValue, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
