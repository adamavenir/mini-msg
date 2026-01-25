package command

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewThreadCmd creates the container thread command.
func NewThreadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thread <path> [anchor]",
		Short: "View or create threads",
		Long: `View an existing thread or create a new one.

If the thread exists, displays its messages.
If the thread doesn't exist, creates it with optional anchor message.

Examples:
  fray thread design-thread                    # View existing thread
  fray thread new-design "Project summary"     # Create with anchor
  fray thread opus/notes                       # View or create path-based thread`,
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			// Try to resolve as existing thread first
			thread, err := resolveThreadRef(ctx.DB, args[0])
			if err != nil {
				// Thread doesn't exist - create it
				return createThreadFromPath(cmd, ctx, args)
			}

			pinnedOnly, _ := cmd.Flags().GetBool("pinned")
			lastStr, _ := cmd.Flags().GetString("last")
			sinceStr, _ := cmd.Flags().GetString("since")
			showAllMessages, _ := cmd.Flags().GetBool("show-all")

			var messages []types.Message
			if pinnedOnly {
				messages, err = db.GetPinnedMessages(ctx.DB, thread.GUID)
			} else {
				messages, err = db.GetThreadMessages(ctx.DB, thread.GUID)
			}
			if err != nil {
				return writeCommandError(cmd, err)
			}
			messages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, messages)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			messages = filterDeletedMessages(messages)

			// Apply --since filter
			if sinceStr != "" {
				cursor, err := core.ParseTimeExpression(ctx.DB, sinceStr, "since")
				if err != nil {
					return writeCommandError(cmd, err)
				}
				var filtered []types.Message
				for _, msg := range messages {
					if cursor.GUID != "" && msg.ID > cursor.GUID {
						filtered = append(filtered, msg)
					} else if cursor.GUID == "" && msg.TS > cursor.TS {
						filtered = append(filtered, msg)
					}
				}
				messages = filtered
			}

			// Apply --last limit
			if lastStr != "" {
				limit, err := strconv.Atoi(lastStr)
				if err != nil {
					return writeCommandError(cmd, fmt.Errorf("invalid --last value: %s", lastStr))
				}
				if limit > 0 && len(messages) > limit {
					messages = messages[len(messages)-limit:]
				}
			}

			path, err := buildThreadPath(ctx.DB, thread)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Get anchor message if set
			var anchorMsg *types.Message
			if thread.AnchorMessageGUID != nil {
				anchorMsg, err = db.GetMessage(ctx.DB, *thread.AnchorMessageGUID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread":   thread,
					"path":     path,
					"messages": messages,
					"anchor":   anchorMsg,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Thread %s (%s) [%s]\n", path, thread.GUID, thread.Status)

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			projectName := GetProjectName(ctx.Project.Root)

			// Display anchor at top with thread metadata tree (full content, no truncation)
			if anchorMsg != nil {
				fmt.Fprintln(out)
				fmt.Fprintf(out, "%sANCHOR:%s\n", dim, reset)
				fmt.Fprintln(out, FormatMessageFull(*anchorMsg, projectName, bases))

				// Build thread metadata tree
				participants := collectParticipants(messages)
				pinnedCount, err := db.GetPinnedMessageCount(ctx.DB, thread.GUID)
				lastActivity := formatLastActivity(thread.LastActivityAt)

				fmt.Fprintf(out, "  └── %s\n", strings.Join(participants, ", "))
				if err == nil && pinnedCount > 0 {
					fmt.Fprintf(out, "  └── %d messages (%d pinned)\n", len(messages), pinnedCount)
				} else {
					fmt.Fprintf(out, "  └── %d messages\n", len(messages))
				}
				fmt.Fprintf(out, "  └── last: %s\n", lastActivity)
				fmt.Fprintln(out)

				// Filter anchor from messages to avoid duplication
				messages = filterMessage(messages, anchorMsg.ID)
			}
			fmt.Fprintln(out)

			if len(messages) == 0 {
				fmt.Fprintln(out, "No messages in thread")
				return nil
			}

			quotedMsgs := CollectQuotedMessages(ctx.DB, messages)

			// Build set of pinned message GUIDs for accordion expansion
			pinnedGUIDs := make(map[string]bool)
			if pinnedMsgs, err := db.GetPinnedMessages(ctx.DB, thread.GUID); err == nil {
				for _, pm := range pinnedMsgs {
					pinnedGUIDs[pm.ID] = true
				}
			}

			lines := FormatMessageListAccordion(messages, AccordionOptions{
				ShowAll:     showAllMessages,
				ProjectName: projectName,
				AgentBases:  bases,
				QuotedMsgs:  quotedMsgs,
				PinnedGUIDs: pinnedGUIDs,
			})
			for _, line := range lines {
				fmt.Fprintln(out, line)
			}
			return nil
		},
	}

	cmd.Flags().Bool("pinned", false, "show only pinned messages")
	cmd.Flags().String("last", "", "show last N messages")
	cmd.Flags().String("since", "", "show messages after time or GUID")
	cmd.Flags().Bool("show-all", false, "disable accordion, show all messages fully")
	cmd.Flags().String("as", "", "agent to attribute anchor message (for creation)")
	cmd.Flags().String("subscribe", "", "comma-separated agent list to subscribe (for creation)")

	cmd.AddCommand(
		NewThreadRenameCmd(),
		NewThreadPinCmd(),
		NewThreadUnpinCmd(),
	)

	return cmd
}

// createThreadFromPath creates a thread from a path specification.
// Supports paths like "design-thread" or "opus/notes" with optional anchor.
func createThreadFromPath(cmd *cobra.Command, ctx *CommandContext, args []string) error {
	pathArg := args[0]
	var anchorText string
	if len(args) > 1 {
		anchorText = args[1]
	}

	asRef, _ := cmd.Flags().GetString("as")
	subscribeList, _ := cmd.Flags().GetString("subscribe")

	// Parse path to determine parent and thread name
	var parentGUID *string
	var name string

	if strings.Contains(pathArg, "/") {
		parts := strings.Split(pathArg, "/")
		name = parts[len(parts)-1]

		// Resolve parent path
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		parent, err := resolveThreadRef(ctx.DB, parentPath)
		if err != nil {
			return writeCommandError(cmd, fmt.Errorf("parent thread not found: %s", parentPath))
		}
		parentGUID = &parent.GUID

		// Check nesting depth
		parentDepth, err := getThreadDepth(ctx.DB, parent)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		if parentDepth >= MaxThreadNestingDepth {
			return writeCommandError(cmd, fmt.Errorf("cannot create thread: maximum nesting depth (%d) exceeded", MaxThreadNestingDepth))
		}
	} else {
		name = pathArg
	}

	name = strings.TrimSpace(name)
	if err := validateThreadName(name); err != nil {
		return writeCommandError(cmd, err)
	}

	// Sanitize name to kebab-case and confirm if changed
	sanitized, changed := SanitizeThreadName(name)
	if sanitized == "" {
		return writeCommandError(cmd, fmt.Errorf("invalid thread name: '%s'", name))
	}
	if changed {
		confirmed, err := ConfirmSanitizedName(name, sanitized, os.Stdout, os.Stdin)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		name = confirmed
	} else {
		name = sanitized
	}

	// Check if thread already exists
	existing, err := db.GetThreadByName(ctx.DB, name, parentGUID)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	if existing != nil {
		return writeCommandError(cmd, fmt.Errorf("thread already exists: %s", pathArg))
	}

	// Check for meta/ path collision (e.g., creating "opus/notes" when "meta/opus/notes" exists)
	if err := CheckMetaPathCollisionForCreate(ctx.DB, parentGUID, name); err != nil {
		return writeCommandError(cmd, err)
	}

	// Create the thread
	thread, err := db.CreateThread(ctx.DB, types.Thread{
		Name:         name,
		ParentThread: parentGUID,
		Status:       types.ThreadStatusOpen,
	})
	if err != nil {
		return writeCommandError(cmd, err)
	}

	// Handle subscribers
	subscribers := splitCommaList(subscribeList)
	for i, subscriber := range subscribers {
		subscribers[i] = ResolveAgentRef(subscriber, ctx.ProjectConfig)
	}

	if err := db.AppendThread(ctx.Project.DBPath, thread, subscribers); err != nil {
		return writeCommandError(cmd, err)
	}

	now := time.Now().Unix()
	for _, agentID := range subscribers {
		if err := db.SubscribeThread(ctx.DB, thread.GUID, agentID, now); err != nil {
			return writeCommandError(cmd, err)
		}
	}

	// Create anchor message if provided
	var anchorGUID string
	if anchorText != "" {
		agentID := "system"
		if asRef != "" {
			agentID, err = resolveAgentRef(ctx, asRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}
		}

		bases, err := db.GetAgentBases(ctx.DB)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		mentions := core.ExtractMentions(anchorText, bases)
		mentions = core.ExpandAllMention(mentions, bases)

		newMsg := types.Message{
			TS:        now,
			Home:      thread.GUID,
			FromAgent: agentID,
			Body:      anchorText,
			Mentions:  mentions,
			Type:      types.MessageTypeAgent,
		}

		created, err := db.CreateMessage(ctx.DB, newMsg)
		if err != nil {
			return writeCommandError(cmd, err)
		}

		if err := db.AppendMessage(ctx.Project.DBPath, created); err != nil {
			return writeCommandError(cmd, err)
		}

		anchorGUID = created.ID

		// Set as anchor
		_, err = db.UpdateThread(ctx.DB, thread.GUID, db.ThreadUpdates{
			AnchorMessageGUID: types.OptionalString{Set: true, Value: &anchorGUID},
			LastActivityAt:    types.OptionalInt64{Set: true, Value: &now},
		})
		if err != nil {
			return writeCommandError(cmd, err)
		}

		if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
			GUID:              thread.GUID,
			AnchorMessageGUID: &anchorGUID,
			LastActivityAt:    &now,
		}); err != nil {
			return writeCommandError(cmd, err)
		}
	}

	if ctx.JSONMode {
		payload := map[string]any{
			"thread":     thread,
			"subscribed": subscribers,
			"anchor":     anchorGUID,
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}

	path, _ := buildThreadPath(ctx.DB, &thread)
	if anchorText != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Created thread %s (%s) with anchor\n", path, thread.GUID)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Created thread %s (%s)\n", path, thread.GUID)
	}
	return nil
}
