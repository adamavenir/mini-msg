package command

import (
	"encoding/json"
	"fmt"
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
		Use:   "thread [ref]",
		Short: "View or manage threads",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return cmd.Help()
			}
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			thread, err := resolveThreadRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
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

			lines := FormatMessageListAccordion(messages, AccordionOptions{
				ShowAll:     showAllMessages,
				ProjectName: projectName,
				AgentBases:  bases,
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

	cmd.AddCommand(
		NewThreadNewCmd(),
		NewThreadAddCmd(),
		NewThreadRemoveCmd(),
		NewThreadSubscribeCmd(),
		NewThreadUnsubscribeCmd(),
		NewThreadArchiveCmd(),
		NewThreadRestoreCmd(),
		NewThreadRenameCmd(),
		NewThreadAnchorCmd(),
		NewThreadPinCmd(),
		NewThreadUnpinCmd(),
		NewThreadMuteCmd(),
		NewThreadUnmuteCmd(),
	)

	return cmd
}

// NewThreadsCmd creates the threads list command.
func NewThreadsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "threads",
		Short: "List threads",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			all, _ := cmd.Flags().GetBool("all")
			pinnedOnly, _ := cmd.Flags().GetBool("pinned")
			mutedOnly, _ := cmd.Flags().GetBool("muted")
			asRef, _ := cmd.Flags().GetString("as")

			// Handle --pinned filter
			if pinnedOnly {
				threads, err := db.GetPinnedThreads(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				return outputThreads(cmd, ctx, threads, "Pinned threads:")
			}

			// Handle --muted filter
			if mutedOnly {
				agentID, err := resolveSubscriptionAgent(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				threads, err := db.GetMutedThreads(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				return outputThreads(cmd, ctx, threads, "Muted threads:")
			}

			var options types.ThreadQueryOptions
			var agentID string
			if all {
				options.IncludeArchived = true
			} else {
				agentID, err = resolveSubscriptionAgent(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				options.SubscribedAgent = &agentID
			}

			threads, err := db.GetThreads(ctx.DB, &options)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Exclude muted threads by default (unless --all or --muted)
			if !all && agentID != "" {
				mutedGUIDs, err := db.GetMutedThreadGUIDs(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if len(mutedGUIDs) > 0 {
					filtered := make([]types.Thread, 0, len(threads))
					for _, t := range threads {
						if !mutedGUIDs[t.GUID] {
							filtered = append(filtered, t)
						}
					}
					threads = filtered
				}
			}

			return outputThreads(cmd, ctx, threads, "Threads:")
		},
	}

	cmd.Flags().Bool("all", false, "list all threads (includes muted)")
	cmd.Flags().Bool("pinned", false, "list only pinned threads")
	cmd.Flags().Bool("muted", false, "list only muted threads")
	cmd.Flags().String("as", "", "agent or user to list subscriptions for")

	return cmd
}

func outputThreads(cmd *cobra.Command, ctx *CommandContext, threads []types.Thread, header string) error {
	if ctx.JSONMode {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(threads)
	}

	out := cmd.OutOrStdout()
	if len(threads) == 0 {
		fmt.Fprintln(out, "No threads found")
		return nil
	}
	fmt.Fprintln(out, header)
	for _, thread := range threads {
		path, err := buildThreadPath(ctx.DB, &thread)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		// Check if thread is pinned for display
		pinned, _ := db.IsThreadPinned(ctx.DB, thread.GUID)
		indicator := ""
		if pinned {
			indicator = " [pinned]"
		}
		fmt.Fprintf(out, "  %s (%s) [%s]%s\n", path, thread.GUID, thread.Status, indicator)
	}
	return nil
}

func resolveSubscriptionAgent(ctx *CommandContext, ref string) (string, error) {
	if ref != "" {
		return ResolveAgentRef(ref, ctx.ProjectConfig), nil
	}
	username, err := db.GetConfig(ctx.DB, "username")
	if err != nil {
		return "", err
	}
	if username == "" {
		return "", fmt.Errorf("--as is required unless --all is set")
	}
	return username, nil
}

// NewThreadNewCmd creates the thread new command.
func NewThreadNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			name := strings.TrimSpace(args[0])
			if err := validateThreadName(name); err != nil {
				return writeCommandError(cmd, err)
			}

			parentRef, _ := cmd.Flags().GetString("parent")
			var parent *types.Thread
			if parentRef != "" {
				parent, err = resolveThreadRef(ctx.DB, parentRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			var parentGUID *string
			if parent != nil {
				parentGUID = &parent.GUID
			}

			existing, err := db.GetThreadByName(ctx.DB, name, parentGUID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if existing != nil {
				return writeCommandError(cmd, fmt.Errorf("thread already exists: %s", name))
			}

			thread, err := db.CreateThread(ctx.DB, types.Thread{
				Name:         name,
				ParentThread: parentGUID,
				Status:       types.ThreadStatusOpen,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			subscribeList, _ := cmd.Flags().GetString("subscribe")
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

			if ctx.JSONMode {
				payload := map[string]any{
					"thread":     thread,
					"subscribed": subscribers,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created thread %s (%s)\n", name, thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("parent", "", "parent thread guid or path")
	cmd.Flags().String("subscribe", "", "comma-separated agent list to subscribe")

	return cmd
}

// NewThreadAddCmd creates the thread add command.
func NewThreadAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <thread> <message...>",
		Short: "Add messages to a thread",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			thread, err := resolveThreadRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			addedBy := "system"
			if asRef, _ := cmd.Flags().GetString("as"); asRef != "" {
				agentID, err := resolveAgentRef(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				addedBy = agentID
			}

			now := time.Now().Unix()
			added := 0
			for _, messageRef := range args[1:] {
				msg, err := resolveMessageRef(ctx.DB, messageRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if msg.Home == thread.GUID {
					continue
				}
				inThread, err := db.IsMessageInThread(ctx.DB, thread.GUID, msg.ID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if inThread {
					continue
				}
				if err := db.AddMessageToThread(ctx.DB, thread.GUID, msg.ID, addedBy, now); err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendThreadMessage(ctx.Project.DBPath, db.ThreadMessageJSONLRecord{
					ThreadGUID:  thread.GUID,
					MessageGUID: msg.ID,
					AddedBy:     addedBy,
					AddedAt:     now,
				}); err != nil {
					return writeCommandError(cmd, err)
				}
				added++
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread": thread.GUID,
					"added":  added,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added %d message(s) to %s\n", added, thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to attribute the add")

	return cmd
}

// NewThreadRemoveCmd creates the thread remove command.
func NewThreadRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <thread> <message...>",
		Short: "Remove messages from a thread",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			thread, err := resolveThreadRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			removedBy := "system"
			if asRef, _ := cmd.Flags().GetString("as"); asRef != "" {
				agentID, err := resolveAgentRef(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				removedBy = agentID
			}

			now := time.Now().Unix()
			removed := 0
			for _, messageRef := range args[1:] {
				msg, err := resolveMessageRef(ctx.DB, messageRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if msg.Home == thread.GUID {
					return writeCommandError(cmd, fmt.Errorf("message %s has home %s and cannot be removed", msg.ID, thread.GUID))
				}
				if err := db.RemoveMessageFromThread(ctx.DB, thread.GUID, msg.ID); err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendThreadMessageRemove(ctx.Project.DBPath, db.ThreadMessageRemoveJSONLRecord{
					ThreadGUID:  thread.GUID,
					MessageGUID: msg.ID,
					RemovedBy:   removedBy,
					RemovedAt:   now,
				}); err != nil {
					return writeCommandError(cmd, err)
				}
				removed++
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread":  thread.GUID,
					"removed": removed,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed %d message(s) from %s\n", removed, thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to attribute the removal")

	return cmd
}

// NewThreadSubscribeCmd creates the thread subscribe command.
func NewThreadSubscribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subscribe <thread>",
		Short: "Subscribe to a thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			thread, err := resolveThreadRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			agentRef, _ := cmd.Flags().GetString("agent")
			agentID, err := resolveSubscriptionAgent(ctx, agentRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			if err := db.SubscribeThread(ctx.DB, thread.GUID, agentID, now); err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.AppendThreadSubscribe(ctx.Project.DBPath, db.ThreadSubscribeJSONLRecord{
				ThreadGUID:   thread.GUID,
				AgentID:      agentID,
				SubscribedAt: now,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread": thread.GUID,
					"agent":  agentID,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Subscribed %s to %s\n", agentID, thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("agent", "", "agent to subscribe")

	return cmd
}

// NewThreadUnsubscribeCmd creates the thread unsubscribe command.
func NewThreadUnsubscribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unsubscribe <thread>",
		Short: "Unsubscribe from a thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			thread, err := resolveThreadRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			agentRef, _ := cmd.Flags().GetString("agent")
			agentID, err := resolveSubscriptionAgent(ctx, agentRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.UnsubscribeThread(ctx.DB, thread.GUID, agentID); err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.AppendThreadUnsubscribe(ctx.Project.DBPath, db.ThreadUnsubscribeJSONLRecord{
				ThreadGUID:     thread.GUID,
				AgentID:        agentID,
				UnsubscribedAt: time.Now().Unix(),
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread": thread.GUID,
					"agent":  agentID,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Unsubscribed %s from %s\n", agentID, thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("agent", "", "agent to unsubscribe")

	return cmd
}

// NewThreadArchiveCmd creates the thread archive command.
func NewThreadArchiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive <thread>",
		Short: "Archive a thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateThreadStatus(cmd, args[0], types.ThreadStatusArchived)
		},
	}
	return cmd
}

// NewThreadRestoreCmd creates the thread restore command.
func NewThreadRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <thread>",
		Short: "Restore a thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateThreadStatus(cmd, args[0], types.ThreadStatusOpen)
		},
	}
	return cmd
}

func updateThreadStatus(cmd *cobra.Command, ref string, status types.ThreadStatus) error {
	ctx, err := GetContext(cmd)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	defer ctx.DB.Close()

	thread, err := resolveThreadRef(ctx.DB, ref)
	if err != nil {
		return writeCommandError(cmd, err)
	}

	statusValue := string(status)
	updated, err := db.UpdateThread(ctx.DB, thread.GUID, db.ThreadUpdates{
		Status: types.OptionalString{Set: true, Value: &statusValue},
	})
	if err != nil {
		return writeCommandError(cmd, err)
	}

	if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
		GUID:   updated.GUID,
		Status: &statusValue,
	}); err != nil {
		return writeCommandError(cmd, err)
	}

	if ctx.JSONMode {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(updated)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Thread %s set to %s\n", updated.GUID, updated.Status)
	return nil
}

// NewThreadRenameCmd creates the thread rename command.
func NewThreadRenameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <thread> <name>",
		Short: "Rename a thread",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			thread, err := resolveThreadRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			name := strings.TrimSpace(args[1])
			if err := validateThreadName(name); err != nil {
				return writeCommandError(cmd, err)
			}

			var parentGUID *string
			if thread.ParentThread != nil {
				parentGUID = thread.ParentThread
			}
			existing, err := db.GetThreadByName(ctx.DB, name, parentGUID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if existing != nil && existing.GUID != thread.GUID {
				return writeCommandError(cmd, fmt.Errorf("thread already exists: %s", name))
			}

			updated, err := db.UpdateThread(ctx.DB, thread.GUID, db.ThreadUpdates{
				Name: types.OptionalString{Set: true, Value: &name},
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
				GUID: updated.GUID,
				Name: &name,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(updated)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Renamed thread %s to %s\n", updated.GUID, updated.Name)
			return nil
		},
	}

	return cmd
}
