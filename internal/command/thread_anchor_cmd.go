package command

import (
	"encoding/json"
	"fmt"
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
