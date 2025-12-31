package command

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewGetCmd creates the get command.
func NewGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [agent]",
		Short: "Get messages",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			last, _ := cmd.Flags().GetString("last")
			since, _ := cmd.Flags().GetString("since")
			before, _ := cmd.Flags().GetString("before")
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			all, _ := cmd.Flags().GetBool("all")
			room, _ := cmd.Flags().GetString("room")
			mentions, _ := cmd.Flags().GetString("mentions")
			unfiltered, _ := cmd.Flags().GetBool("unfiltered")
			archived, _ := cmd.Flags().GetBool("archived")
			hideEvents, _ := cmd.Flags().GetBool("hide-events")
			showEvents, _ := cmd.Flags().GetBool("show-events")
			if showEvents {
				hideEvents = false
			}

			isQueryMode := last != "" || since != "" || before != "" || from != "" || to != "" || all

			projectName := GetProjectName(ctx.Project.Root)
			var agentBases map[string]struct{}
			if !ctx.JSONMode {
				agentBases, err = db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}
			var resolvedAgentID string
			if len(args) > 0 {
				resolvedAgentID, err = resolveAgentRef(ctx, args[0])
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			var filter *types.Filter
			if resolvedAgentID != "" && !unfiltered {
				filter, err = db.GetFilter(ctx.DB, resolvedAgentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if isQueryMode {
				var options types.MessageQueryOptions
				options.Filter = filter
				options.IncludeArchived = archived

				if all {
					// no limits
				} else if since != "" || before != "" || from != "" || to != "" {
					if since != "" && from != "" {
						return writeCommandError(cmd, fmt.Errorf("use --since or --from, not both"))
					}
					if before != "" && to != "" {
						return writeCommandError(cmd, fmt.Errorf("use --before or --to, not both"))
					}

					start := since
					if start == "" {
						start = from
					}
					end := before
					if end == "" {
						end = to
					}

					if start != "" {
						cursor, err := core.ParseTimeExpression(ctx.DB, start, "since")
						if err != nil {
							return writeCommandError(cmd, err)
						}
						options.Since = cursor
					}
					if end != "" {
						cursor, err := core.ParseTimeExpression(ctx.DB, end, "before")
						if err != nil {
							return writeCommandError(cmd, err)
						}
						options.Before = cursor
					}
				} else {
					limit, err := strconv.Atoi(last)
					if err != nil {
						return writeCommandError(cmd, fmt.Errorf("invalid --last value"))
					}
					options.Limit = limit
				}

				messages, err := db.GetMessages(ctx.DB, &options)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				messages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, messages)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if hideEvents {
					messages = filterEventMessages(messages)
				}

				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(messages)
				}

				out := cmd.OutOrStdout()
				if len(messages) == 0 {
					fmt.Fprintln(out, "No messages")
					return nil
				}
				for _, msg := range messages {
					fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
				}
				return nil
			}

			if resolvedAgentID != "" {
				roomLimit := parseOptionalInt(room, 10)
				mentionsLimit := parseOptionalInt(mentions, 3)

				roomMessages, err := db.GetMessages(ctx.DB, &types.MessageQueryOptions{Limit: roomLimit, Filter: filter, IncludeArchived: archived})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				roomMessages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, roomMessages)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if hideEvents {
					roomMessages = filterEventMessages(roomMessages)
				}

				agentBase := resolvedAgentID
				if strings.Contains(resolvedAgentID, ".") {
					idx := strings.LastIndex(resolvedAgentID, ".")
					agentBase = resolvedAgentID[:idx]
				}

				mentionMessages, err := db.GetMessagesWithMention(ctx.DB, agentBase, &types.MessageQueryOptions{
					Limit:           mentionsLimit + roomLimit,
					IncludeArchived: archived,
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				mentionMessages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, mentionMessages)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if hideEvents {
					mentionMessages = filterEventMessages(mentionMessages)
				}

				roomIDs := map[string]struct{}{}
				for _, msg := range roomMessages {
					roomIDs[msg.ID] = struct{}{}
				}

				filtered := make([]types.Message, 0, len(mentionMessages))
				for _, msg := range mentionMessages {
					if _, ok := roomIDs[msg.ID]; ok {
						continue
					}
					filtered = append(filtered, msg)
					if len(filtered) == mentionsLimit {
						break
					}
				}

				if len(filtered) > 0 {
					ids := make([]string, 0, len(filtered))
					for _, msg := range filtered {
						ids = append(ids, msg.ID)
					}
					if err := db.MarkMessagesRead(ctx.DB, ids, agentBase); err != nil {
						return writeCommandError(cmd, err)
					}
				}

				if ctx.JSONMode {
					payload := map[string]any{
						"project":       projectName,
						"room_messages": roomMessages,
						"mentions":      filtered,
					}
					return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
				}

				out := cmd.OutOrStdout()
				if len(roomMessages) == 0 {
					fmt.Fprintln(out, "ROOM: (no messages yet)")
				} else {
					fmt.Fprintln(out, "ROOM:")
					for _, msg := range roomMessages {
						fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
					}
				}

				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "---")
				fmt.Fprintln(out, "")

				if len(filtered) == 0 {
					fmt.Fprintf(out, "@%s: (no additional mentions)\n", agentBase)
				} else {
					fmt.Fprintf(out, "@%s:\n", agentBase)
					for _, msg := range filtered {
						fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
						for _, reactionLine := range formatReactionEvents(msg) {
							fmt.Fprintf(out, "  %s\n", reactionLine)
						}
					}
				}

				claims, err := db.GetAllClaims(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
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

					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "---")
					fmt.Fprintf(out, "Active claims: %s\n", strings.Join(claimParts, ", "))
				}

				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "---")
				fmt.Fprintf(out, "More: fray get --last 50 | fray @%s --all | fray get --since <guid>\n", agentBase)
				return nil
			}

			return writeCommandError(cmd, fmt.Errorf("usage: fray get <agent>        Combined room + @mentions view\n       fray get --last <n>     Last N messages\n       fray get --since <guid> Messages after GUID\n       fray get --all          All messages"))
		},
	}

	cmd.Flags().String("last", "", "show last N messages")
	cmd.Flags().String("since", "", "show messages after time or GUID")
	cmd.Flags().String("before", "", "show messages before time or GUID")
	cmd.Flags().String("from", "", "range start (time or GUID)")
	cmd.Flags().String("to", "", "range end (time or GUID)")
	cmd.Flags().Bool("all", false, "show all messages")
	cmd.Flags().String("room", "10", "number of room messages in combined view")
	cmd.Flags().String("mentions", "3", "number of @mentions in combined view")
	cmd.Flags().Bool("unfiltered", false, "bypass saved filter, show all messages")
	cmd.Flags().Bool("archived", false, "include archived messages")
	cmd.Flags().Bool("hide-events", false, "hide event messages")
	cmd.Flags().Bool("show-events", false, "show event messages")

	return cmd
}

func parseOptionalInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
