package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
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

			prefix, err := resolveAgentRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}
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
			messages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, messages)
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
			} else {
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
					for _, reactionLine := range formatReactionEvents(msg) {
						fmt.Fprintf(out, "  %s\n", reactionLine)
					}
				}

				ids := make([]string, 0, len(messages))
				for _, msg := range messages {
					ids = append(ids, msg.ID)
				}
				if err := db.MarkMessagesRead(ctx.DB, ids, prefix); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if !ctx.JSONMode {
				if err := printActivitySummary(out, ctx.DB, prefix); err != nil {
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

func printActivitySummary(out interface{ Write([]byte) (int, error) }, database *sql.DB, agentPrefix string) error {
	lastPostTS, err := getLastPostTimestamp(database, agentPrefix)
	if err != nil {
		return err
	}

	claims, err := db.GetAllClaims(database)
	if err != nil {
		return err
	}

	messagesSince := int64(0)
	senderSet := make(map[string]struct{})

	if lastPostTS != nil {
		rows, err := database.Query(`
			SELECT from_agent FROM fray_messages
			WHERE ts > ? AND archived_at IS NULL AND from_agent != ?
		`, *lastPostTS, agentPrefix)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var sender string
			if err := rows.Scan(&sender); err != nil {
				return err
			}
			messagesSince++
			senderSet[sender] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}

	if messagesSince == 0 && len(claims) == 0 {
		return nil
	}

	fmt.Fprintf(out, "\n---\n")

	if messagesSince > 0 {
		senders := make([]string, 0, len(senderSet))
		for sender := range senderSet {
			senders = append(senders, "@"+sender)
		}

		senderList := ""
		if len(senders) > 0 {
			senderList = " (from " + strings.Join(senders, ", ") + ")"
		}

		fmt.Fprintf(out, "%d message", messagesSince)
		if messagesSince > 1 {
			fmt.Fprintf(out, "s")
		}
		fmt.Fprintf(out, " since you last posted%s\n", senderList)
	}

	if len(claims) > 0 {
		claimsByAgent := make(map[string][]string)
		for _, claim := range claims {
			pattern := claim.Pattern
			if claim.ClaimType != types.ClaimTypeFile {
				pattern = fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern)
			}
			claimsByAgent[claim.AgentID] = append(claimsByAgent[claim.AgentID], pattern)
		}

		claimParts := make([]string, 0, len(claimsByAgent))
		for agentID, patterns := range claimsByAgent {
			claimParts = append(claimParts, fmt.Sprintf("@%s (%s)", agentID, strings.Join(patterns, ", ")))
		}
		fmt.Fprintf(out, "Active claims: %s\n", strings.Join(claimParts, ", "))
	}

	fmt.Fprintf(out, "Run 'fray get %s' to catch up\n", agentPrefix)

	return nil
}

func getLastPostTimestamp(database *sql.DB, agentPrefix string) (*int64, error) {
	row := database.QueryRow(`
		SELECT MAX(ts) FROM fray_messages
		WHERE from_agent = ? AND archived_at IS NULL
	`, agentPrefix)

	var ts sql.NullInt64
	if err := row.Scan(&ts); err != nil {
		return nil, err
	}

	if !ts.Valid {
		return nil, nil
	}

	value := ts.Int64
	return &value, nil
}
