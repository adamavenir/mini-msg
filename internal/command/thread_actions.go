package command

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

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

			parentRef, _ := cmd.Flags().GetString("parent")
			var parent *types.Thread
			if parentRef != "" {
				parent, err = resolveThreadRef(ctx.DB, parentRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				// Check nesting depth - new thread would be parent's depth + 1
				parentDepth, err := getThreadDepth(ctx.DB, parent)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if parentDepth >= MaxThreadNestingDepth {
					return writeCommandError(cmd, fmt.Errorf("cannot create thread: maximum nesting depth (%d) exceeded", MaxThreadNestingDepth))
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

			// Check for meta/ path collision
			if err := CheckMetaPathCollisionForCreate(ctx.DB, parentGUID, name); err != nil {
				return writeCommandError(cmd, err)
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

			// Create event for thread rename (shows names, not IDs)
			agentRef, _ := cmd.Flags().GetString("as")
			if agentRef == "" {
				agentRef = os.Getenv("FRAY_AGENT_ID")
			}
			if agentRef != "" {
				agentID, err := resolveAgentRef(ctx, agentRef)
				if err == nil {
					eventBody := fmt.Sprintf("edited %s → %s", thread.Name, name)
					eventMessage, err := db.CreateMessage(ctx.DB, types.Message{
						TS:        time.Now().Unix(),
						FromAgent: agentID,
						Body:      eventBody,
						Type:      types.MessageTypeEvent,
						Home:      "room",
					})
					if err == nil {
						_ = db.AppendMessage(ctx.Project.DBPath, eventMessage)
					}
				}
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(updated)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Renamed thread %s to %s\n", thread.Name, updated.Name)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent to attribute the rename event (uses FRAY_AGENT_ID if not set)")
	return cmd
}
