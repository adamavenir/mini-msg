package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewPostCmd creates the post command.
func NewPostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "post",
		Short: "Post message to room",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			replyTo, _ := cmd.Flags().GetString("reply-to")
			silent, _ := cmd.Flags().GetBool("silent")

			if agentRef == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required"))
			}
			agentID := ResolveAgentRef(agentRef, ctx.ProjectConfig)

			agent, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agent == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found: @%s. Use 'mm new' first", agentID))
			}
			if agent.LeftAt != nil {
				return writeCommandError(cmd, fmt.Errorf("agent @%s has left. Use 'mm back @%s' to resume", agentID, agentID))
			}

			var replyID *string
			if replyTo != "" {
				trimmed := strings.TrimSpace(strings.TrimPrefix(replyTo, "#"))
				msg, err := db.GetMessage(ctx.DB, trimmed)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if msg == nil {
					return writeCommandError(cmd, fmt.Errorf("message %s not found", trimmed))
				}
				replyID = &trimmed
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			mentions := core.ExtractMentions(args[0], bases)

			now := time.Now().Unix()
			created, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: agentID,
				Body:      args[0],
				Mentions:  mentions,
				ReplyTo:   replyID,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendMessage(ctx.Project.DBPath, created); err != nil {
				return writeCommandError(cmd, err)
			}

			updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
			if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
				return writeCommandError(cmd, err)
			}

			if silent {
				return nil
			}

			agentBase := agentID
			if parsed, err := core.ParseAgentID(agentID); err == nil {
				agentBase = parsed.Base
			}

			unread, err := db.GetMessagesWithMention(ctx.DB, agentBase, &types.MessageQueryOptions{UnreadOnly: true, AgentPrefix: agentBase})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			filtered := make([]types.Message, 0, len(unread))
			for _, msg := range unread {
				parsed, err := core.ParseAgentID(msg.FromAgent)
				if err != nil {
					filtered = append(filtered, msg)
					continue
				}
				if parsed.Base != agentBase {
					filtered = append(filtered, msg)
				}
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"id":       created.ID,
					"from":     agentID,
					"mentions": mentions,
					"reply_to": replyID,
					"unread":   len(filtered),
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			replyInfo := ""
			if replyID != nil {
				replyInfo = fmt.Sprintf(" (reply to #%s)", *replyID)
			}
			fmt.Fprintf(out, "[%s] Posted as @%s%s\n", created.ID, agentID, replyInfo)

			if len(filtered) > 0 {
				fmt.Fprintf(out, "\n%d unread @%s:\n", len(filtered), agentBase)
				previewCount := 0
				for _, msg := range filtered {
					preview := msg.Body
					if len(preview) > 60 {
						preview = preview[:60] + "..."
					}
					fmt.Fprintf(out, "  [%s] %s: %s\n", msg.ID, msg.FromAgent, preview)
					previewCount++
					if previewCount == 5 {
						break
					}
				}
				if len(filtered) > 5 {
					fmt.Fprintf(out, "  ... and %d more\n", len(filtered)-5)
				}
			}

			if len(filtered) > 0 {
				ids := make([]string, 0, len(filtered))
				for _, msg := range filtered {
					ids = append(ids, msg.ID)
				}
				if err := db.MarkMessagesRead(ctx.DB, ids, agentBase); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to post as")
	cmd.Flags().StringP("reply-to", "r", "", "reply to message GUID (threading)")
	cmd.Flags().BoolP("silent", "s", false, "suppress output including unread mentions")

	_ = cmd.MarkFlagRequired("as")

	return cmd
}
