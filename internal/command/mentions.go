package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// Staleness threshold: messages older than this are "likely stale"
const staleThresholdHours = 2

// mentionCategory represents how an agent was mentioned
type mentionCategory int

const (
	categoryDirectAddress mentionCategory = iota // @agent at message start
	categoryFYI                                  // @agent mid-message
	categoryReply                                // reply to agent's message
)

// categorizedMessage wraps a message with its mention metadata
type categorizedMessage struct {
	types.Message
	category mentionCategory
	isStale  bool
	ageStr   string
}

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
			showAllMessages, _ := cmd.Flags().GetBool("show-all")

			prefix, err := resolveAgentRef(ctx, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}
			// Include mentions from all locations (room + threads)
			// Also include replies to agent's messages (chained replies)
			allHomes := ""
			options := &types.MessageQueryOptions{
				IncludeArchived:       includeArchived,
				AgentPrefix:           prefix,
				Home:                  &allHomes,
				IncludeRepliesToAgent: prefix,
			}

			// Check ghost cursor for session-aware unread logic
			var ghostCursor *types.GhostCursor
			useGhostCursorBoundary := false

			unreadOnly := true
			if showAll {
				// --all: show everything
				unreadOnly = false
				options.Limit = 0
			} else if since != "" {
				// --since: show from specific message
				unreadOnly = false
				options.SinceID = strings.TrimPrefix(strings.TrimPrefix(since, "@"), "#")
			} else {
				// Default: check ghost cursor for unread boundary
				ghostCursor, _ = db.GetGhostCursor(ctx.DB, prefix, "room")
				if ghostCursor != nil && ghostCursor.SessionAckAt == nil {
					// Ghost cursor exists and not yet acked this session
					// Use ghost cursor as the unread boundary instead of read receipts
					msg, err := db.GetMessage(ctx.DB, ghostCursor.MessageGUID)
					if err == nil && msg != nil {
						options.Since = &types.MessageCursor{GUID: msg.ID, TS: msg.TS}
						useGhostCursorBoundary = true
						unreadOnly = false // Don't also filter by read receipts
					}
				}
				// Apply limit if specified (--last N), applies to both ghost cursor and read receipt modes
				if limit > 0 {
					options.Limit = limit
				}
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
			// For display: unread if using read receipts OR ghost cursor boundary
			isUnreadMode := unreadOnly || useGhostCursorBoundary
			if len(messages) == 0 {
				if isUnreadMode {
					fmt.Fprintf(out, "No unread mentions of @%s\n", prefix)
				} else {
					fmt.Fprintf(out, "No mentions of @%s\n", prefix)
				}
				// Ack ghost cursor even if no messages (boundary viewed)
				if useGhostCursorBoundary && ghostCursor != nil {
					now := time.Now().Unix()
					_ = db.AckGhostCursor(ctx.DB, prefix, "room", now)
				}
			} else {
				bases, err := db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				projectName := GetProjectName(ctx.Project.Root)
				now := time.Now()

				// Categorize all messages
				categorized := make([]categorizedMessage, len(messages))
				for i, msg := range messages {
					categorized[i] = categorizeMessage(msg, prefix, now)
				}

				// Use hierarchical display for unread mode, flat for --all/--since
				if isUnreadMode {
					printHierarchicalMentions(out, categorized, prefix, projectName)
				} else {
					// Flat display for explicit queries
					fmt.Fprintf(out, "Messages mentioning @%s:\n", prefix)

					// Apply accordion if needed
					threshold := DefaultAccordionThreshold
					useAccordion := !showAllMessages && len(messages) > threshold
					headCount := AccordionHeadCount
					tailCount := AccordionTailCount

					for i, msg := range messages {
						// Determine if this is a preview (collapsed) or full message
						isPreview := false
						if useAccordion {
							middleStart := headCount
							middleEnd := len(messages) - tailCount
							if i >= middleStart && i < middleEnd {
								isPreview = true
							}
							// Print accordion markers
							if i == middleStart {
								collapsedCount := middleEnd - middleStart
								fmt.Fprintf(out, "%s  ... %d messages collapsed ...%s\n", dim, collapsedCount, reset)
							}
						}

						if isPreview {
							fmt.Fprintln(out, FormatMessagePreview(msg, projectName))
						} else {
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

						// Print end of accordion marker
						if useAccordion && i == len(messages)-tailCount-1 {
							fmt.Fprintf(out, "%s  ... end collapsed ...%s\n", dim, reset)
						}
					}
				}

				ids := make([]string, 0, len(messages))
				for _, msg := range messages {
					ids = append(ids, msg.ID)
				}
				if err := db.MarkMessagesRead(ctx.DB, ids, prefix); err != nil {
					return writeCommandError(cmd, err)
				}

				// Ack ghost cursor if we used it as boundary (first view this session)
				if useGhostCursorBoundary && ghostCursor != nil {
					nowTS := time.Now().Unix()
					if err := db.AckGhostCursor(ctx.DB, prefix, "room", nowTS); err != nil {
						return writeCommandError(cmd, err)
					}
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
	cmd.Flags().Bool("show-all", false, "disable accordion, show all messages fully")

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

// isDirectAddress checks if the message body starts with an @mention of the agent
func isDirectAddress(body, agentPrefix string) bool {
	// Match @agent or @agent.* at the very start of the message
	pattern := fmt.Sprintf(`^@%s(?:\.\w+)*\b`, regexp.QuoteMeta(agentPrefix))
	matched, _ := regexp.MatchString(pattern, strings.TrimSpace(body))
	return matched
}

// categorizeMessage determines how an agent was mentioned in a message
func categorizeMessage(msg types.Message, agentPrefix string, now time.Time) categorizedMessage {
	cat := categorizedMessage{Message: msg}

	// Calculate age
	msgTime := time.Unix(msg.TS, 0)
	age := now.Sub(msgTime)
	cat.isStale = age.Hours() >= staleThresholdHours
	cat.ageStr = formatAge(age)

	// Determine category
	if msg.ReplyTo != nil {
		// This is a reply - check if it's a reply to the agent's message
		// (we assume the query already filtered for this)
		cat.category = categoryReply
	} else if isDirectAddress(msg.Body, agentPrefix) {
		cat.category = categoryDirectAddress
	} else {
		cat.category = categoryFYI
	}

	return cat
}

// formatAge returns a human-readable age string
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	} else if d < time.Hour {
		mins := int(d.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd ago", days)
}

// printHierarchicalMentions displays mentions grouped by category and staleness
func printHierarchicalMentions(out interface{ Write([]byte) (int, error) }, messages []categorizedMessage, agentPrefix, projectName string) {
	// Group messages
	var directRecent, directStale, fyiRecent, fyiStale, replyRecent, replyStale []categorizedMessage

	for _, msg := range messages {
		switch msg.category {
		case categoryDirectAddress:
			if msg.isStale {
				directStale = append(directStale, msg)
			} else {
				directRecent = append(directRecent, msg)
			}
		case categoryFYI:
			if msg.isStale {
				fyiStale = append(fyiStale, msg)
			} else {
				fyiRecent = append(fyiRecent, msg)
			}
		case categoryReply:
			if msg.isStale {
				replyStale = append(replyStale, msg)
			} else {
				replyRecent = append(replyRecent, msg)
			}
		}
	}

	printed := false

	// Print recent direct addresses (highest priority)
	if len(directRecent) > 0 {
		fmt.Fprintf(out, "Recent @%s:\n", agentPrefix)
		for _, msg := range directRecent {
			printMentionLine(out, msg, projectName)
		}
		printed = true
	}

	// Print recent FYIs
	if len(fyiRecent) > 0 {
		if printed {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, "You were FYI'd here:")
		for _, msg := range fyiRecent {
			printMentionLine(out, msg, projectName)
		}
		printed = true
	}

	// Print recent replies
	if len(replyRecent) > 0 {
		if printed {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, "Replies to your messages:")
		for _, msg := range replyRecent {
			printMentionLine(out, msg, projectName)
		}
		printed = true
	}

	// Count stale
	staleCount := len(directStale) + len(fyiStale) + len(replyStale)
	if staleCount > 0 {
		if printed {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "%s%d likely stale (>%dh)%s\n", dim, staleCount, staleThresholdHours, reset)
	}
}

// printMentionLine prints a single mention in compact format
func printMentionLine(out interface{ Write([]byte) (int, error) }, msg categorizedMessage, projectName string) {
	// Truncate body for display
	body := strings.TrimSpace(msg.Body)
	body = strings.ReplaceAll(body, "\n", " ")
	maxLen := 60
	if len(body) > maxLen {
		body = body[:maxLen-3] + "..."
	}

	fmt.Fprintf(out, "  [%s] %s: %s (%s)\n", formatShortGUID(msg.ID), msg.FromAgent, body, msg.ageStr)
}

// formatShortGUID returns a shortened GUID for display
func formatShortGUID(guid string) string {
	if strings.HasPrefix(guid, "msg-") && len(guid) > 7 {
		return guid[:7]
	}
	return guid
}
