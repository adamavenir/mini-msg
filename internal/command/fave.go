package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewFaveCmd creates the fave command.
func NewFaveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fave <item>",
		Short: "Fave a thread or message",
		Long:  "Add an item to your faves. Faving a thread also subscribes you to it.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			if agentRef == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required"))
			}

			agentID, err := resolveAgentRef(ctx, agentRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s. Use 'fray new' first", agentID))
			}
			if agent.LeftAt != nil {
				return writeCommandError(cmd, fmt.Errorf("agent @%s has left. Use 'fray back @%s' to resume", agentID, agentID))
			}

			itemRef := args[0]
			itemType, itemGUID, err := resolveItemRef(ctx.DB, itemRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Check if already faved
			already, err := db.IsFaved(ctx.DB, agentID, itemType, itemGUID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if already {
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"already_faved": true,
						"item_type":     itemType,
						"item_guid":     itemGUID,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Already faved #%s\n", itemGUID)
				return nil
			}

			// Add fave
			favedAt, err := db.AddFave(ctx.DB, agentID, itemType, itemGUID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Write to JSONL
			if err := db.AppendAgentFave(ctx.Project.DBPath, agentID, itemType, itemGUID, favedAt); err != nil {
				return writeCommandError(cmd, err)
			}

			// If faving a thread, also subscribe (idempotent - INSERT OR REPLACE)
			if itemType == "thread" {
				subscribedAt := time.Now().Unix()
				if err := db.SubscribeThread(ctx.DB, itemGUID, agentID, subscribedAt); err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendThreadSubscribe(ctx.Project.DBPath, db.ThreadSubscribeJSONLRecord{
					ThreadGUID:   itemGUID,
					AgentID:      agentID,
					SubscribedAt: subscribedAt,
				}); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			// Update last_seen
			now := time.Now().Unix()
			updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"faved":     true,
					"item_type": itemType,
					"item_guid": itemGUID,
					"faved_at":  favedAt,
				})
			}

			typeLabel := itemType
			if itemType == "thread" {
				typeLabel = "thread"
			} else {
				typeLabel = "message"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Faved %s #%s\n", typeLabel, itemGUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to fave as")
	_ = cmd.MarkFlagRequired("as")

	return cmd
}

// NewUnfaveCmd creates the unfave command.
func NewUnfaveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unfave <item>",
		Short: "Unfave a thread or message",
		Long:  "Remove an item from your faves. Note: unfaving a thread does NOT unsubscribe you (use mute for that).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			if agentRef == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required"))
			}

			agentID, err := resolveAgentRef(ctx, agentRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s. Use 'fray new' first", agentID))
			}

			itemRef := args[0]
			itemType, itemGUID, err := resolveItemRef(ctx.DB, itemRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			// Check if faved
			faved, err := db.IsFaved(ctx.DB, agentID, itemType, itemGUID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if !faved {
				if ctx.JSONMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
						"not_faved": true,
						"item_type": itemType,
						"item_guid": itemGUID,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Not faved: #%s\n", itemGUID)
				return nil
			}

			// Remove fave
			if err := db.RemoveFave(ctx.DB, agentID, itemType, itemGUID); err != nil {
				return writeCommandError(cmd, err)
			}

			// Write to JSONL
			unfavedAt := time.Now().UnixMilli()
			if err := db.AppendAgentUnfave(ctx.Project.DBPath, agentID, itemType, itemGUID, unfavedAt); err != nil {
				return writeCommandError(cmd, err)
			}

			// Update last_seen
			now := time.Now().Unix()
			updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"unfaved":    true,
					"item_type":  itemType,
					"item_guid":  itemGUID,
					"unfaved_at": unfavedAt,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Unfaved #%s\n", itemGUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to unfave as")
	_ = cmd.MarkFlagRequired("as")

	return cmd
}

// NewFavesCmd creates the faves listing command.
func NewFavesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "faves",
		Short: "List your faved items",
		Long:  "Show all threads and messages you have faved.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			if agentRef == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required"))
			}

			agentID, err := resolveAgentRef(ctx, agentRef)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			threadsOnly, _ := cmd.Flags().GetBool("threads")
			messagesOnly, _ := cmd.Flags().GetBool("messages")

			itemType := ""
			if threadsOnly && !messagesOnly {
				itemType = "thread"
			} else if messagesOnly && !threadsOnly {
				itemType = "message"
			}

			faves, err := db.GetFaves(ctx.DB, agentID, itemType)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(faves)
			}

			if len(faves) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No faves\n")
				return nil
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Faves (%d items)\n\n", len(faves))

			for _, f := range faves {
				prefix := "msg"
				if f.ItemType == "thread" {
					prefix = "thrd"
				}
				fmt.Fprintf(out, "  [%s] %s-%s\n", f.ItemType, prefix, strings.TrimPrefix(f.ItemGUID, prefix+"-"))
			}

			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to list faves for")
	cmd.Flags().Bool("threads", false, "show only faved threads")
	cmd.Flags().Bool("messages", false, "show only faved messages")

	_ = cmd.MarkFlagRequired("as")

	return cmd
}

// resolveItemRef determines if a reference is a thread or message and returns the type and GUID.
func resolveItemRef(dbConn *sql.DB, ref string) (itemType string, itemGUID string, err error) {
	// Try thread first (thrd- prefix or name lookup)
	if strings.HasPrefix(ref, "thrd-") || !strings.HasPrefix(ref, "msg-") {
		thread, threadErr := resolveThreadRef(dbConn, ref)
		if threadErr == nil && thread != nil {
			return "thread", thread.GUID, nil
		}
	}

	// Try message
	msg, msgErr := resolveMessageRef(dbConn, ref)
	if msgErr == nil && msg != nil {
		return "message", msg.ID, nil
	}

	// If neither worked, return a helpful error
	if strings.HasPrefix(ref, "thrd-") {
		return "", "", fmt.Errorf("thread not found: %s", ref)
	}
	if strings.HasPrefix(ref, "msg-") {
		return "", "", fmt.Errorf("message not found: %s", ref)
	}
	return "", "", fmt.Errorf("item not found: %s (try thrd-<id> or msg-<id>)", ref)
}
