package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewFollowCmd creates the follow command (top-level subscribe).
func NewFollowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "follow <thread>",
		Short: "Follow a thread",
		Long: `Follow a thread to receive notifications.

Accepts thread GUID, name, or path.

Examples:
  fray follow design-thread
  fray follow opus/notes
  fray follow thrd-xyz --as alice`,
		Args: cobra.ExactArgs(1),
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

			agentRef, _ := cmd.Flags().GetString("as")
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

			path, _ := buildThreadPath(ctx.DB, thread)
			if path == "" {
				path = thread.GUID
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Following %s\n", path)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent to follow as")

	return cmd
}

// NewUnfollowCmd creates the unfollow command (top-level unsubscribe).
func NewUnfollowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unfollow <thread>",
		Short: "Unfollow a thread",
		Long: `Unfollow a thread to stop receiving notifications.

Accepts thread GUID, name, or path.

Examples:
  fray unfollow design-thread
  fray unfollow opus/notes
  fray unfollow thrd-xyz --as alice`,
		Args: cobra.ExactArgs(1),
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

			agentRef, _ := cmd.Flags().GetString("as")
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

			path, _ := buildThreadPath(ctx.DB, thread)
			if path == "" {
				path = thread.GUID
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unfollowed %s\n", path)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent to unfollow as")

	return cmd
}

// NewMuteCmd creates the mute command (top-level thread mute).
func NewMuteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mute <thread>",
		Short: "Mute a thread",
		Long: `Mute a thread to suppress notifications.

Accepts thread GUID, name, or path.

Examples:
  fray mute design-thread
  fray mute opus/notes --ttl 2h
  fray mute thrd-xyz --as alice`,
		Args: cobra.ExactArgs(1),
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

			asRef, _ := cmd.Flags().GetString("as")
			ttlStr, _ := cmd.Flags().GetString("ttl")

			if asRef == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required"))
			}

			agentID, err := resolveAgentRef(ctx, asRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			var expiresAt *int64
			if ttlStr != "" {
				seconds, err := parseDuration(ttlStr)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				expiryTime := now + seconds
				expiresAt = &expiryTime
			}

			if err := db.MuteThread(ctx.DB, thread.GUID, agentID, now, expiresAt); err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendThreadMute(ctx.Project.DBPath, db.ThreadMuteJSONLRecord{
				ThreadGUID: thread.GUID,
				AgentID:    agentID,
				MutedAt:    now,
				ExpiresAt:  expiresAt,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread":     thread.GUID,
					"muted":      true,
					"expires_at": expiresAt,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			path, _ := buildThreadPath(ctx.DB, thread)
			if path == "" {
				path = thread.GUID
			}
			if expiresAt != nil {
				expiryStr := time.Unix(*expiresAt, 0).Format("2006-01-02 15:04")
				fmt.Fprintf(cmd.OutOrStdout(), "Muted %s until %s\n", path, expiryStr)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Muted %s\n", path)
			}
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent muting the thread (required)")
	cmd.Flags().String("ttl", "", "mute duration (e.g., 2h, 1d)")

	return cmd
}

// NewUnmuteCmd creates the unmute command (top-level thread unmute).
func NewUnmuteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unmute <thread>",
		Short: "Unmute a thread",
		Long: `Unmute a thread to resume notifications.

Accepts thread GUID, name, or path.

Examples:
  fray unmute design-thread
  fray unmute opus/notes
  fray unmute thrd-xyz --as alice`,
		Args: cobra.ExactArgs(1),
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

			asRef, _ := cmd.Flags().GetString("as")

			if asRef == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required"))
			}

			agentID, err := resolveAgentRef(ctx, asRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.UnmuteThread(ctx.DB, thread.GUID, agentID); err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			if err := db.AppendThreadUnmute(ctx.Project.DBPath, db.ThreadUnmuteJSONLRecord{
				ThreadGUID: thread.GUID,
				AgentID:    agentID,
				UnmutedAt:  now,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread":  thread.GUID,
					"unmuted": true,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			path, _ := buildThreadPath(ctx.DB, thread)
			if path == "" {
				path = thread.GUID
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unmuted %s\n", path)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent unmuting the thread (required)")

	return cmd
}

// NewAddCmd creates the add command (add messages to thread).
func NewAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <thread> <message...>",
		Short: "Add messages to a thread",
		Long: `Add messages to a thread without moving them.

Accepts thread GUID, name, or path.

Examples:
  fray add design-thread msg-abc
  fray add opus/notes msg-xyz msg-def`,
		Args: cobra.MinimumNArgs(2),
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

			path, _ := buildThreadPath(ctx.DB, thread)
			if path == "" {
				path = thread.GUID
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added %d message(s) to %s\n", added, path)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to attribute the add")

	return cmd
}

// NewRemoveCmd creates the remove command (remove messages from thread).
func NewRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <thread> <message...>",
		Short: "Remove messages from a thread",
		Long: `Remove messages from a thread.

Accepts thread GUID, name, or path.

Examples:
  fray remove design-thread msg-abc
  fray remove opus/notes msg-xyz`,
		Args: cobra.MinimumNArgs(2),
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

			path, _ := buildThreadPath(ctx.DB, thread)
			if path == "" {
				path = thread.GUID
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %d message(s) from %s\n", removed, path)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to attribute the removal")

	return cmd
}

// NewArchiveCmd creates the archive command (archive threads).
func NewArchiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive <thread>",
		Short: "Archive a thread",
		Long: `Archive a thread.

Accepts thread GUID, name, or path.

Examples:
  fray archive design-thread
  fray archive opus/notes --as opus`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateThreadStatusTopLevel(cmd, args[0], "archived")
		},
	}

	cmd.Flags().String("as", "", "agent performing the archive (for attribution)")

	return cmd
}

// NewRestoreCmd creates the restore command (restore archived threads).
func NewRestoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore <thread>",
		Short: "Restore an archived thread",
		Long: `Restore an archived thread.

Accepts thread GUID, name, or path.

Examples:
  fray restore design-thread
  fray restore opus/notes --as opus`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateThreadStatusTopLevel(cmd, args[0], "open")
		},
	}

	cmd.Flags().String("as", "", "agent performing the restore (for attribution)")

	return cmd
}

// updateThreadStatusTopLevel updates thread status for top-level commands.
func updateThreadStatusTopLevel(cmd *cobra.Command, ref string, status string) error {
	ctx, err := GetContext(cmd)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	defer ctx.DB.Close()

	thread, err := resolveThreadRef(ctx.DB, ref)
	if err != nil {
		return writeCommandError(cmd, err)
	}

	updated, err := db.UpdateThread(ctx.DB, thread.GUID, db.ThreadUpdates{
		Status: types.OptionalString{Set: true, Value: &status},
	})
	if err != nil {
		return writeCommandError(cmd, err)
	}

	if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
		GUID:   updated.GUID,
		Status: &status,
	}); err != nil {
		return writeCommandError(cmd, err)
	}

	if ctx.JSONMode {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(updated)
	}

	path, _ := buildThreadPath(ctx.DB, thread)
	if path == "" {
		path = thread.GUID
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Thread %s set to %s\n", path, updated.Status)
	return nil
}

// NewAnchorCmd creates the anchor command (set thread anchor).
func NewAnchorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "anchor <thread> [message-or-text]",
		Short: "Set or update thread anchor",
		Long: `Set or update a thread's anchor message.

The anchor serves as a TL;DR for the thread. Accepts thread GUID, name, or path.

Examples:
  fray anchor design-thread msg-abc
  fray anchor opus/notes "Summary of notes"
  fray anchor design-thread --hide`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Delegate to the existing thread anchor implementation
			return NewThreadAnchorCmd().RunE(cmd, args)
		},
	}

	cmd.Flags().Bool("hide", false, "hide anchor from parent thread")
	cmd.Flags().Bool("unhide", false, "show anchor in parent thread")
	cmd.Flags().String("as", "", "agent to attribute new anchor message")

	return cmd
}
