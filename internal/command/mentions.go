package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewMentionsCmd creates the mentions command.
func NewMentionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mentions <agent>",
		Short: "Show messages mentioning an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			limit, _ := cmd.Flags().GetInt("last")
			since, _ := cmd.Flags().GetString("since")
			showAll, _ := cmd.Flags().GetBool("all")
			includeArchived, _ := cmd.Flags().GetBool("archived")

			prefix := ResolveAgentRef(args[0], ctx.ProjectConfig)
			options := &types.MessageQueryOptions{IncludeArchived: includeArchived, AgentPrefix: prefix}

			unreadOnly := true
			if showAll {
				unreadOnly = false
				options.Limit = 0
			} else if since != "" {
				unreadOnly = false
				options.SinceID = strings.TrimPrefix(strings.TrimPrefix(since, "@"), "#")
			} else if limit > 0 {
				options.Limit = limit
			}
			options.UnreadOnly = unreadOnly

			messages, err := db.GetMessagesWithMention(ctx.DB, prefix, options)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(messages)
			}

			out := cmd.OutOrStdout()
			if len(messages) == 0 {
				if unreadOnly {
					fmt.Fprintf(out, "No unread mentions of @%s\n", prefix)
				} else {
					fmt.Fprintf(out, "No mentions of @%s\n", prefix)
				}
				return nil
			}

			if unreadOnly {
				fmt.Fprintf(out, "Unread mentions of @%s:\n", prefix)
			} else {
				fmt.Fprintf(out, "Messages mentioning @%s:\n", prefix)
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			projectName := GetProjectName(ctx.Project.Root)
			for _, msg := range messages {
				formatted := FormatMessage(msg, projectName, bases)
				readCount, err := db.GetReadReceiptCount(ctx.DB, msg.ID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if readCount > 0 {
					formatted = strings.TrimRight(formatted, "\n")
					formatted = fmt.Sprintf("%s [✓%d]", formatted, readCount)
				}
				fmt.Fprintln(out, formatted)
			}

			if len(messages) > 0 {
				ids := make([]string, 0, len(messages))
				for _, msg := range messages {
					ids = append(ids, msg.ID)
				}
				if err := db.MarkMessagesRead(ctx.DB, ids, prefix); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			return nil
		},
	}

	cmd.Flags().Int("last", 20, "show last N mentions")
	cmd.Flags().String("since", "", "show mentions since message ID")
	cmd.Flags().Bool("all", false, "show all mentions")
	cmd.Flags().Bool("archived", false, "include archived messages")

	return cmd
}
