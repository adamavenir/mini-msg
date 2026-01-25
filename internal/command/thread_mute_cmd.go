package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// NewThreadPinCmd creates the thread pin command.
func NewThreadPinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pin <thread>",
		Short: "Pin a thread (public)",
		Long: `Pin a thread to highlight it for everyone.

Pinned threads appear in fray threads --pinned listing.

Examples:
  fray thread pin thrd-xyz --as alice`,
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
			pinnedBy := "system"
			if asRef != "" {
				pinnedBy, err = resolveAgentRef(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			now := time.Now().Unix()
			if err := db.PinThread(ctx.DB, thread.GUID, pinnedBy, now); err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendThreadPin(ctx.Project.DBPath, db.ThreadPinJSONLRecord{
				ThreadGUID: thread.GUID,
				PinnedBy:   pinnedBy,
				PinnedAt:   now,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread": thread.GUID,
					"pinned": true,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Pinned thread %s\n", thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent to attribute the pin")

	return cmd
}

// NewThreadUnpinCmd creates the thread unpin command.
func NewThreadUnpinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unpin <thread>",
		Short: "Unpin a thread",
		Long: `Unpin a thread.

Examples:
  fray thread unpin thrd-xyz --as alice`,
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
			unpinnedBy := "system"
			if asRef != "" {
				unpinnedBy, err = resolveAgentRef(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if err := db.UnpinThread(ctx.DB, thread.GUID); err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			if err := db.AppendThreadUnpin(ctx.Project.DBPath, db.ThreadUnpinJSONLRecord{
				ThreadGUID: thread.GUID,
				UnpinnedBy: unpinnedBy,
				UnpinnedAt: now,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"thread":   thread.GUID,
					"unpinned": true,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Unpinned thread %s\n", thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent to attribute the unpin")

	return cmd
}

// NewThreadMuteCmd creates the thread mute command.
func NewThreadMuteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mute <thread>",
		Short: "Mute a thread",
		Long: `Mute a thread to unsubscribe from notifications.

Muted threads are excluded from default thread listings.
Use --ttl to set a temporary mute with expiration.

Examples:
  fray thread mute thrd-xyz --as alice
  fray thread mute thrd-xyz --as alice --ttl 2h`,
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

			if expiresAt != nil {
				expiryStr := time.Unix(*expiresAt, 0).Format("2006-01-02 15:04")
				fmt.Fprintf(cmd.OutOrStdout(), "Muted thread %s until %s\n", thread.GUID, expiryStr)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Muted thread %s\n", thread.GUID)
			}
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent muting the thread (required)")
	cmd.Flags().String("ttl", "", "mute duration (e.g., 2h, 1d)")

	return cmd
}

// NewThreadUnmuteCmd creates the thread unmute command.
func NewThreadUnmuteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unmute <thread>",
		Short: "Unmute a thread",
		Long: `Unmute a thread to resume notifications.

Examples:
  fray thread unmute thrd-xyz --as alice`,
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

			fmt.Fprintf(cmd.OutOrStdout(), "Unmuted thread %s\n", thread.GUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent unmuting the thread (required)")

	return cmd
}
