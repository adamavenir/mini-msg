package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

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
