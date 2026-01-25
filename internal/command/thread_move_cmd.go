package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

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

					// Remove message from all thread playlists (it's being moved, not copied)
					memberships, err := db.GetMessageThreadMemberships(ctx.DB, m.ID)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					for _, threadGUID := range memberships {
						if err := db.RemoveMessageFromThread(ctx.DB, threadGUID, m.ID); err != nil {
							return writeCommandError(cmd, err)
						}
						if err := db.AppendThreadMessageRemove(ctx.Project.DBPath, db.ThreadMessageRemoveJSONLRecord{
							ThreadGUID:  threadGUID,
							MessageGUID: m.ID,
							RemovedBy:   movedBy,
							RemovedAt:   now,
						}); err != nil {
							return writeCommandError(cmd, err)
						}
					}

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
