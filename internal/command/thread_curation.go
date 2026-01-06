package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewThreadAnchorCmd creates the thread anchor command.
func NewThreadAnchorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "anchor <thread> [message-or-text]",
		Short: "Set or update thread anchor",
		Long: `Set or update a thread's anchor message.

The anchor serves as a TL;DR for the thread. It can be:
- An existing message GUID/prefix
- New text (creates a new message as anchor)

Examples:
  fray thread anchor thrd-xyz msg-abc      # Use existing message as anchor
  fray thread anchor thrd-xyz "Summary"    # Create new message as anchor
  fray thread anchor thrd-xyz --hide       # Hide anchor from parent thread
  fray thread anchor thrd-xyz --unhide     # Show anchor in parent thread`,
		Args: cobra.RangeArgs(1, 2),
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

			hide, _ := cmd.Flags().GetBool("hide")
			unhide, _ := cmd.Flags().GetBool("unhide")
			asRef, _ := cmd.Flags().GetString("as")

			// Handle hide/unhide toggles
			if hide || unhide {
				newHidden := hide
				updated, err := db.UpdateThread(ctx.DB, thread.GUID, db.ThreadUpdates{
					AnchorHidden: types.OptionalBool{Set: true, Value: newHidden},
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}

				if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
					GUID:         updated.GUID,
					AnchorHidden: &newHidden,
				}); err != nil {
					return writeCommandError(cmd, err)
				}

				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(updated)
				}

				if newHidden {
					fmt.Fprintf(cmd.OutOrStdout(), "Anchor hidden from parent thread\n")
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Anchor visible in parent thread\n")
				}
				return nil
			}

			// Need message or text
			if len(args) < 2 {
				return writeCommandError(cmd, fmt.Errorf("message GUID or anchor text required"))
			}

			messageOrText := args[1]
			var anchorGUID string

			// Try to resolve as message first
			msg, err := resolveMessageRef(ctx.DB, messageOrText)
			if err == nil && msg != nil {
				anchorGUID = msg.ID
			} else {
				// Create new message as anchor
				agentID := "system"
				if asRef != "" {
					agentID, err = resolveAgentRef(ctx, asRef)
					if err != nil {
						return writeCommandError(cmd, err)
					}
				}

				now := time.Now().Unix()
				bases, err := db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				mentions := core.ExtractMentions(messageOrText, bases)
				mentions = core.ExpandAllMention(mentions, bases)

				newMsg := types.Message{
					TS:        now,
					Home:      thread.GUID,
					FromAgent: agentID,
					Body:      messageOrText,
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

				// Update thread activity
				if err := db.UpdateThreadActivity(ctx.DB, thread.GUID, now); err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
					GUID:           thread.GUID,
					LastActivityAt: &now,
				}); err != nil {
					return writeCommandError(cmd, err)
				}

				anchorGUID = created.ID
			}

			// Update thread anchor
			updated, err := db.UpdateThread(ctx.DB, thread.GUID, db.ThreadUpdates{
				AnchorMessageGUID: types.OptionalString{Set: true, Value: &anchorGUID},
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
				GUID:              updated.GUID,
				AnchorMessageGUID: &anchorGUID,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(updated)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Set anchor for %s to %s\n", thread.GUID, anchorGUID)
			return nil
		},
	}

	cmd.Flags().Bool("hide", false, "hide anchor from parent thread")
	cmd.Flags().Bool("unhide", false, "show anchor in parent thread")
	cmd.Flags().String("as", "", "agent to attribute new anchor message")

	return cmd
}

// NewPinCmd creates the pin command.
func NewPinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pin <message> [--thread <thread>]",
		Short: "Pin a message",
		Long: `Pin a message within a thread.

Messages can be pinned in specific threads. If --thread is not specified,
the message is pinned in its home thread.

Examples:
  fray pin msg-abc                        # Pin in message's home thread
  fray pin msg-abc --thread thrd-xyz      # Pin in specific thread`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			msg, err := resolveMessageRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			threadRef, _ := cmd.Flags().GetString("thread")
			asRef, _ := cmd.Flags().GetString("as")

			// Determine thread
			var threadGUID string
			if threadRef != "" {
				thread, err := resolveThreadRef(ctx.DB, threadRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				threadGUID = thread.GUID
			} else if msg.Home != "room" {
				threadGUID = msg.Home
			} else {
				return writeCommandError(cmd, fmt.Errorf("message is in room; specify --thread"))
			}

			// Resolve agent
			pinnedBy := "system"
			if asRef != "" {
				pinnedBy, err = resolveAgentRef(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			now := time.Now().Unix()
			if err := db.PinMessage(ctx.DB, msg.ID, threadGUID, pinnedBy, now); err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendMessagePin(ctx.Project.DBPath, db.MessagePinJSONLRecord{
				MessageGUID: msg.ID,
				ThreadGUID:  threadGUID,
				PinnedBy:    pinnedBy,
				PinnedAt:    now,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"message": msg.ID,
					"thread":  threadGUID,
					"pinned":  true,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Pinned %s in %s\n", msg.ID, threadGUID)
			return nil
		},
	}

	cmd.Flags().String("thread", "", "thread to pin in")
	cmd.Flags().String("as", "", "agent to attribute the pin")

	return cmd
}

// NewUnpinCmd creates the unpin command.
func NewUnpinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unpin <message> [--thread <thread>]",
		Short: "Unpin a message",
		Long: `Unpin a message from a thread.

If --thread is not specified, unpins from the message's home thread.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			msg, err := resolveMessageRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			threadRef, _ := cmd.Flags().GetString("thread")
			asRef, _ := cmd.Flags().GetString("as")

			// Determine thread
			var threadGUID string
			if threadRef != "" {
				thread, err := resolveThreadRef(ctx.DB, threadRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				threadGUID = thread.GUID
			} else if msg.Home != "room" {
				threadGUID = msg.Home
			} else {
				return writeCommandError(cmd, fmt.Errorf("message is in room; specify --thread"))
			}

			// Resolve agent
			unpinnedBy := "system"
			if asRef != "" {
				unpinnedBy, err = resolveAgentRef(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if err := db.UnpinMessage(ctx.DB, msg.ID, threadGUID); err != nil {
				return writeCommandError(cmd, err)
			}

			now := time.Now().Unix()
			if err := db.AppendMessageUnpin(ctx.Project.DBPath, db.MessageUnpinJSONLRecord{
				MessageGUID: msg.ID,
				ThreadGUID:  threadGUID,
				UnpinnedBy:  unpinnedBy,
				UnpinnedAt:  now,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"message":  msg.ID,
					"thread":   threadGUID,
					"unpinned": true,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Unpinned %s from %s\n", msg.ID, threadGUID)
			return nil
		},
	}

	cmd.Flags().String("thread", "", "thread to unpin from")
	cmd.Flags().String("as", "", "agent to attribute the unpin")

	return cmd
}

// NewMvCmd creates the mv command.
func NewMvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mv <message|thread> <destination> [anchor]",
		Short: "Move messages or reparent threads",
		Long: `Move messages to a different thread, or reparent a thread under a new parent.

For messages:
  The destination can be a thread reference or "room" to move back to room.
  Use --with-replies to move the message and all its replies.

For threads:
  Move a thread to become a child of another thread.
  Use "root" or "/" as destination to make it a root-level thread.
  Optionally provide anchor text as a third argument.

Examples:
  fray mv msg-abc thrd-xyz               # Move message to thread
  fray mv msg-abc room                   # Move message back to room
  fray mv msg-abc thrd-xyz --with-replies
  fray mv design-thread meta             # Reparent thread under meta
  fray mv design-thread meta "Summary"   # Reparent + set anchor
  fray mv design-thread root             # Make thread root-level`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			withReplies, _ := cmd.Flags().GetBool("with-replies")
			asRef, _ := cmd.Flags().GetString("as")

			// Try to resolve first arg as a thread (for reparenting)
			sourceThread, threadErr := resolveThreadRef(ctx.DB, args[0])
			if threadErr == nil && sourceThread != nil {
				return runThreadReparent(cmd, ctx, args, sourceThread, asRef)
			}

			// Otherwise, treat as message move (existing behavior)
			// Last arg is destination
			destRef := args[len(args)-1]
			messageRefs := args[:len(args)-1]

			// Resolve destination
			var newHome string
			if strings.ToLower(destRef) == "room" {
				newHome = "room"
			} else {
				thread, err := resolveThreadRef(ctx.DB, destRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				newHome = thread.GUID
			}

			// Resolve agent
			movedBy := "system"
			if asRef != "" {
				movedBy, err = resolveAgentRef(ctx, asRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			now := time.Now().Unix()
			moved := 0

			for _, messageRef := range messageRefs {
				msg, err := resolveMessageRef(ctx.DB, messageRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				messagesToMove := []types.Message{*msg}

				// If --with-replies, get all replies recursively
				if withReplies {
					replies, err := getAllReplies(ctx.DB, msg.ID)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					messagesToMove = append(messagesToMove, replies...)
				}

				for _, m := range messagesToMove {
					if m.Home == newHome {
						continue
					}

					oldHome := m.Home
					if err := db.MoveMessage(ctx.DB, m.ID, newHome); err != nil {
						return writeCommandError(cmd, err)
					}

					if err := db.AppendMessageMove(ctx.Project.DBPath, db.MessageMoveJSONLRecord{
						MessageGUID: m.ID,
						OldHome:     oldHome,
						NewHome:     newHome,
						MovedBy:     movedBy,
						MovedAt:     now,
					}); err != nil {
						return writeCommandError(cmd, err)
					}
					moved++
				}
			}

			// Update thread activity if moving to a thread
			if newHome != "room" {
				if err := db.UpdateThreadActivity(ctx.DB, newHome, now); err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
					GUID:           newHome,
					LastActivityAt: &now,
				}); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"destination": newHome,
					"moved":       moved,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Moved %d message(s) to %s\n", moved, newHome)
			return nil
		},
	}

	cmd.Flags().Bool("with-replies", false, "move message and all its replies")
	cmd.Flags().String("as", "", "agent to attribute the move")

	return cmd
}

// runThreadReparent handles moving a thread to a new parent.
func runThreadReparent(cmd *cobra.Command, ctx *CommandContext, args []string, sourceThread *types.Thread, asRef string) error {
	if len(args) < 2 {
		return writeCommandError(cmd, fmt.Errorf("destination required: fray mv <thread> <new-parent> [anchor]"))
	}

	destRef := args[1]
	var anchorText string
	if len(args) >= 3 {
		anchorText = strings.Join(args[2:], " ")
	}

	// Resolve agent
	movedBy := "system"
	if asRef != "" {
		var err error
		movedBy, err = resolveAgentRef(ctx, asRef)
		if err != nil {
			return writeCommandError(cmd, err)
		}
	}

	// Resolve destination (new parent)
	var newParentGUID *string
	destLower := strings.ToLower(destRef)
	if destLower == "root" || destLower == "/" {
		// Move to root (no parent)
		newParentGUID = nil
	} else {
		destThread, err := resolveThreadRef(ctx.DB, destRef)
		if err != nil {
			return writeCommandError(cmd, fmt.Errorf("destination thread not found: %s", destRef))
		}
		newParentGUID = &destThread.GUID

		// Check for cycles: can't move thread under one of its descendants
		isDescendant, err := isAncestorOf(ctx.DB, destThread.GUID, sourceThread.GUID)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		if isDescendant {
			return writeCommandError(cmd, fmt.Errorf("cannot move %s under %s: would create a cycle", sourceThread.Name, destThread.Name))
		}

		// Check nesting depth
		destDepth, err := getThreadDepth(ctx.DB, destThread)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		if destDepth >= MaxThreadNestingDepth {
			return writeCommandError(cmd, fmt.Errorf("cannot move: destination thread is at maximum nesting depth (%d)", MaxThreadNestingDepth))
		}
	}

	// Check if already at target parent
	currentParent := ""
	if sourceThread.ParentThread != nil {
		currentParent = *sourceThread.ParentThread
	}
	targetParent := ""
	if newParentGUID != nil {
		targetParent = *newParentGUID
	}
	if currentParent == targetParent {
		return writeCommandError(cmd, fmt.Errorf("%s is already under that parent", sourceThread.Name))
	}

	now := time.Now().Unix()

	// Update thread parent
	_, err := db.UpdateThread(ctx.DB, sourceThread.GUID, db.ThreadUpdates{
		ParentThread: types.OptionalString{Set: true, Value: newParentGUID},
	})
	if err != nil {
		return writeCommandError(cmd, err)
	}

	// Persist to JSONL
	if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
		GUID:         sourceThread.GUID,
		ParentThread: newParentGUID,
	}); err != nil {
		return writeCommandError(cmd, err)
	}

	// If anchor text provided, create anchor message
	if anchorText != "" {
		bases, err := db.GetAgentBases(ctx.DB)
		if err != nil {
			return writeCommandError(cmd, err)
		}
		mentions := core.ExtractMentions(anchorText, bases)
		mentions = core.ExpandAllMention(mentions, bases)

		newMsg := types.Message{
			TS:        now,
			Home:      sourceThread.GUID,
			FromAgent: movedBy,
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

		// Set as anchor
		_, err = db.UpdateThread(ctx.DB, sourceThread.GUID, db.ThreadUpdates{
			AnchorMessageGUID: types.OptionalString{Set: true, Value: &created.ID},
		})
		if err != nil {
			return writeCommandError(cmd, err)
		}

		if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
			GUID:              sourceThread.GUID,
			AnchorMessageGUID: &created.ID,
		}); err != nil {
			return writeCommandError(cmd, err)
		}
	}

	// Update thread activity
	if err := db.UpdateThreadActivity(ctx.DB, sourceThread.GUID, now); err != nil {
		return writeCommandError(cmd, err)
	}
	if err := db.AppendThreadUpdate(ctx.Project.DBPath, db.ThreadUpdateJSONLRecord{
		GUID:           sourceThread.GUID,
		LastActivityAt: &now,
	}); err != nil {
		return writeCommandError(cmd, err)
	}

	if ctx.JSONMode {
		payload := map[string]any{
			"thread":     sourceThread.GUID,
			"new_parent": newParentGUID,
			"anchor":     anchorText != "",
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}

	destName := "root"
	if newParentGUID != nil {
		destThread, _ := db.GetThread(ctx.DB, *newParentGUID)
		if destThread != nil {
			destName = destThread.Name
		}
	}
	if anchorText != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Moved thread %s under %s (with anchor)\n", sourceThread.Name, destName)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Moved thread %s under %s\n", sourceThread.Name, destName)
	}
	return nil
}

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

// getAllReplies recursively gets all replies to a message.
func getAllReplies(database *sql.DB, messageGUID string) ([]types.Message, error) {
	seen := make(map[string]struct{})
	return getAllRepliesWithGuard(database, messageGUID, seen)
}

func getAllRepliesWithGuard(database *sql.DB, messageGUID string, seen map[string]struct{}) ([]types.Message, error) {
	if _, ok := seen[messageGUID]; ok {
		return nil, nil // already visited, break cycle
	}
	seen[messageGUID] = struct{}{}

	var result []types.Message

	replies, err := db.GetReplies(database, messageGUID)
	if err != nil {
		return nil, err
	}

	for _, reply := range replies {
		if _, ok := seen[reply.ID]; ok {
			continue // skip already visited
		}
		result = append(result, reply)
		nested, err := getAllRepliesWithGuard(database, reply.ID, seen)
		if err != nil {
			return nil, err
		}
		result = append(result, nested...)
	}

	return result, nil
}
