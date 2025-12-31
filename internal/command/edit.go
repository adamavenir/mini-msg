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

// NewEditCmd creates the edit command.
func NewEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <msgid> <message> -m <reason>",
		Short: "Edit a message you posted",
		Args:  cobra.MinimumNArgs(2),
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

			reason, _ := cmd.Flags().GetString("message")
			if strings.TrimSpace(reason) == "" {
				return writeCommandError(cmd, fmt.Errorf("--message (-m) is required"))
			}

			msg, err := resolveMessageRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			msgID := msg.ID
			newBody := strings.Join(args[1:], " ")

			if err := db.EditMessage(ctx.DB, msgID, newBody, agentID); err != nil {
				return writeCommandError(cmd, err)
			}

			updated, err := db.GetMessage(ctx.DB, msgID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if updated == nil {
				return writeCommandError(cmd, fmt.Errorf("message %s not found", msgID))
			}

			update := db.MessageUpdateJSONLRecord{ID: updated.ID, EditedAt: updated.EditedAt, Reason: &reason}
			body := updated.Body
			update.Body = &body
			if err := db.AppendMessageUpdate(ctx.Project.DBPath, update); err != nil {
				return writeCommandError(cmd, err)
			}

			totalCount, err := getTotalMessageCount(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			prefixLength := core.GetDisplayPrefixLength(int(totalCount))
			eventBody := fmt.Sprintf("edited #%s: %s", core.GetGUIDPrefix(updated.ID, prefixLength), reason)
			eventTS := time.Now().Unix()
			if updated.EditedAt != nil {
				eventTS = *updated.EditedAt
			}
			reference := updated.ID
			eventMessage, err := db.CreateMessage(ctx.DB, types.Message{
				TS:         eventTS,
				FromAgent:  agentID,
				Body:       eventBody,
				Type:       types.MessageTypeEvent,
				References: &reference,
				Home:       "room",
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.AppendMessage(ctx.Project.DBPath, eventMessage); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{"id": updated.ID, "edited": true}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Edited message #%s\n", updated.ID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID editing the message")
	cmd.Flags().StringP("message", "m", "", "reason for the edit")
	_ = cmd.MarkFlagRequired("as")
	_ = cmd.MarkFlagRequired("message")

	return cmd
}
