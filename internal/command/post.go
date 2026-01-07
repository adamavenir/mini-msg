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

// NewPostCmd creates the post command.
func NewPostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "post [path] <message>",
		Short: "Post message to room or thread",
		Long: `Post a message to the room or a specific thread.

If a path is provided as first arg, posts to that location.
Paths:
  fray post "msg"                    Post to room (default)
  fray post meta "msg"               Post to project meta
  fray post opus/notes "msg"         Post to agent's notes
  fray post design-thread "msg"      Post to thread by name
  fray post roles/architect/keys "msg"  Post to role's keys`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentRef, _ := cmd.Flags().GetString("as")
			replyTo, _ := cmd.Flags().GetString("reply-to")
			threadRef, _ := cmd.Flags().GetString("thread")
			answerRef, _ := cmd.Flags().GetString("answer")
			quoteRef, _ := cmd.Flags().GetString("quote")
			silent, _ := cmd.Flags().GetBool("silent")

			// Determine path and message body
			var messageBody string
			if len(args) == 2 {
				// First arg is path, second is message
				pathArg := args[0]
				messageBody = args[1]

				// Try to resolve path as thread (only if not already using --thread)
				if threadRef == "" {
					thread, err := resolveThreadRef(ctx.DB, pathArg)
					if err == nil && thread != nil {
						threadRef = thread.GUID
					} else {
						return writeCommandError(cmd, fmt.Errorf("thread not found: %s", pathArg))
					}
				}
			} else {
				messageBody = args[0]
			}

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

			// Check if this is the stored human username
			isHumanUser := false
			if agent == nil {
				storedUsername, _ := db.GetConfig(ctx.DB, "username")
				if storedUsername != "" && storedUsername == agentID {
					isHumanUser = true
				} else {
					return writeCommandError(cmd, fmt.Errorf("agent not found: @%s. Use 'fray new' first", agentID))
				}
			}
			if agent != nil && agent.LeftAt != nil {
				return writeCommandError(cmd, fmt.Errorf("agent @%s has left. Use 'fray back @%s' to resume", agentID, agentID))
			}

			var answerQuestion *types.Question
			if answerRef != "" {
				question, matches, err := matchQuestionForAnswer(ctx.DB, answerRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if len(matches) > 0 {
					if ctx.JSONMode {
						payload := map[string]any{
							"ambiguous": true,
							"matches":   matches,
						}
						return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
					}

					out := cmd.OutOrStdout()
					fmt.Fprintf(out, "Multiple questions match %q:\n", answerRef)
					for _, match := range matches {
						threadLabel := "room"
						if match.ThreadGUID != nil {
							thread, _ := db.GetThread(ctx.DB, *match.ThreadGUID)
							if thread != nil {
								if path, err := buildThreadPath(ctx.DB, thread); err == nil && path != "" {
									threadLabel = path
								} else {
									threadLabel = thread.GUID
								}
							} else {
								threadLabel = *match.ThreadGUID
							}
						}
						fmt.Fprintf(out, "  [%s] %s (%s) %s\n", match.GUID, match.Status, threadLabel, match.Re)
					}
					return nil
				}
				answerQuestion = question
				if answerQuestion.Status == types.QuestionStatusClosed {
					return writeCommandError(cmd, fmt.Errorf("question %s is closed", answerQuestion.GUID))
				}
				if threadRef == "" && answerQuestion.ThreadGUID != nil {
					threadRef = *answerQuestion.ThreadGUID
				}
			}

			var thread *types.Thread
			if threadRef != "" {
				thread, err = resolveThreadRef(ctx.DB, threadRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			var replyID *string
			var replyMsg *types.Message
			if replyTo != "" {
				msg, err := resolveMessageRef(ctx.DB, replyTo)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				replyMsg = msg
				replyID = &msg.ID
			}

			var quoteID *string
			if quoteRef != "" {
				msg, err := resolveMessageRef(ctx.DB, quoteRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				quoteID = &msg.ID
			}

			reactionText := ""
			if replyID != nil && answerRef == "" {
				if reaction, ok := core.NormalizeReactionText(messageBody); ok {
					reactionText = reaction
				}
			}

			if reactionText != "" && replyID != nil {
				updated, reactedAt, err := db.AddReaction(ctx.DB, *replyID, agentID, reactionText)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				// Write reaction to JSONL (new format - separate record)
				if err := db.AppendReaction(ctx.Project.DBPath, *replyID, agentID, reactionText, reactedAt); err != nil {
					return writeCommandError(cmd, err)
				}

				if !isHumanUser {
					now := time.Now().Unix()
					updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
					if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
						return writeCommandError(cmd, err)
					}
				}

				if silent {
					return nil
				}

				if ctx.JSONMode {
					payload := map[string]any{
						"id":         updated.ID,
						"from":       agentID,
						"reaction":   reactionText,
						"reacted_at": reactedAt,
					}
					return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
				}

				fmt.Fprintf(cmd.OutOrStdout(), "Reacted %q to #%s\n", reactionText, updated.ID)
				return nil
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			// Include users in mentionable bases so @username mentions are extracted
			users, _ := db.GetActiveUsers(ctx.DB)
			for _, u := range users {
				bases[u] = struct{}{}
			}
			mentions := core.ExtractMentions(messageBody, bases)
			mentions = core.ExpandAllMention(mentions, bases)

			now := time.Now().Unix()
			home := ""
			if thread != nil {
				home = thread.GUID
			}
			msgType := types.MessageTypeAgent
			if isHumanUser {
				msgType = types.MessageTypeUser
			}
			created, err := db.CreateMessage(ctx.DB, types.Message{
				TS:               now,
				FromAgent:        agentID,
				Body:             messageBody,
				Mentions:         mentions,
				Home:             home,
				ReplyTo:          replyID,
				QuoteMessageGUID: quoteID,
				Type:             msgType,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendMessage(ctx.Project.DBPath, created); err != nil {
				return writeCommandError(cmd, err)
			}

			// Implicit subscription: posting to a thread subscribes the poster
			if thread != nil {
				if err := subscribeAgentToThread(ctx, thread.GUID, agentID, now, "post"); err != nil {
					return writeCommandError(cmd, err)
				}
				// Also subscribe mentioned agents
				for _, mention := range mentions {
					if mention != agentID {
						if err := subscribeAgentToThread(ctx, thread.GUID, mention, now, "mention"); err != nil {
							// Non-fatal: agent may not exist
							continue
						}
					}
				}
			}

			if !isHumanUser {
				updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
				if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			// Extract questions from markdown sections
			sections, _ := core.ExtractQuestionSections(messageBody)
			for _, section := range sections {
				status := types.QuestionStatusOpen
				if section.IsWondering {
					status = types.QuestionStatusUnasked
				}

				var threadGUID *string
				if home != "" && home != "room" {
					threadGUID = &home
				}

				for _, eq := range section.Questions {
					// Convert core.QuestionOption to types.QuestionOption
					var options []types.QuestionOption
					for _, opt := range eq.Options {
						options = append(options, types.QuestionOption{
							Label: opt.Label,
							Pros:  opt.Pros,
							Cons:  opt.Cons,
						})
					}

					// Create question for each target, or one with no target if none specified
					targets := section.Targets
					if len(targets) == 0 {
						targets = []string{""}
					}

					for _, target := range targets {
						var toAgent *string
						if target != "" {
							toAgent = &target
						}

						question, err := db.CreateQuestion(ctx.DB, types.Question{
							Re:         eq.Text,
							FromAgent:  agentID,
							ToAgent:    toAgent,
							Status:     status,
							ThreadGUID: threadGUID,
							AskedIn:    &created.ID,
							Options:    options,
							CreatedAt:  now,
						})
						if err != nil {
							return writeCommandError(cmd, err)
						}
						if err := db.AppendQuestion(ctx.Project.DBPath, question); err != nil {
							return writeCommandError(cmd, err)
						}
					}
				}
			}

			if answerQuestion != nil {
				statusValue := string(types.QuestionStatusAnswered)
				updated, err := db.UpdateQuestion(ctx.DB, answerQuestion.GUID, db.QuestionUpdates{
					Status:     types.OptionalString{Set: true, Value: &statusValue},
					AnsweredIn: types.OptionalString{Set: true, Value: &created.ID},
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendQuestionUpdate(ctx.Project.DBPath, db.QuestionUpdateJSONLRecord{
					GUID:       updated.GUID,
					Status:     &statusValue,
					AnsweredIn: &created.ID,
				}); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if thread != nil && replyMsg != nil && replyMsg.Home != thread.GUID {
				inThread, err := db.IsMessageInThread(ctx.DB, thread.GUID, replyMsg.ID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if !inThread {
					if err := db.AddMessageToThread(ctx.DB, thread.GUID, replyMsg.ID, agentID, now); err != nil {
						return writeCommandError(cmd, err)
					}
					if err := db.AppendThreadMessage(ctx.Project.DBPath, db.ThreadMessageJSONLRecord{
						ThreadGUID:  thread.GUID,
						MessageGUID: replyMsg.ID,
						AddedBy:     agentID,
						AddedAt:     now,
					}); err != nil {
						return writeCommandError(cmd, err)
					}
				}
			}

			if silent {
				return nil
			}

			agentBase := agentID
			if parsed, err := core.ParseAgentID(agentID); err == nil {
				agentBase = parsed.Base
			}

			// Check ghost cursor for session-aware unread logic
			allHomes := ""
			mentionOpts := &types.MessageQueryOptions{
				AgentPrefix:           agentBase,
				Home:                  &allHomes,
				IncludeRepliesToAgent: agentBase,
			}

			useGhostCursorBoundary := false
			var mentionGhostCursor *types.GhostCursor
			mentionGhostCursor, _ = db.GetGhostCursor(ctx.DB, agentBase, "room")
			if mentionGhostCursor != nil && mentionGhostCursor.SessionAckAt == nil {
				// Ghost cursor exists and not yet acked this session
				msg, msgErr := db.GetMessage(ctx.DB, mentionGhostCursor.MessageGUID)
				if msgErr == nil && msg != nil {
					mentionOpts.Since = &types.MessageCursor{GUID: msg.ID, TS: msg.TS}
					useGhostCursorBoundary = true
				}
			}
			if !useGhostCursorBoundary {
				mentionOpts.UnreadOnly = true
			}

			unread, err := db.GetMessagesWithMention(ctx.DB, agentBase, mentionOpts)
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

			// Ack ghost cursor if we used it as boundary (first view this session)
			if useGhostCursorBoundary && mentionGhostCursor != nil {
				now := time.Now().Unix()
				_ = db.AckGhostCursor(ctx.DB, agentBase, "room", now)
			}

			// Show active claims summary
			claims, err := db.GetAllClaims(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if len(claims) > 0 {
				claimsByAgent := make(map[string][]string)
				for _, claim := range claims {
					pattern := claim.Pattern
					if claim.ClaimType != types.ClaimTypeFile {
						pattern = fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern)
					}
					claimsByAgent[claim.AgentID] = append(claimsByAgent[claim.AgentID], pattern)
				}
				claimParts := make([]string, 0, len(claimsByAgent))
				for aid, patterns := range claimsByAgent {
					claimParts = append(claimParts, fmt.Sprintf("@%s (%s)", aid, strings.Join(patterns, ", ")))
				}
				fmt.Fprintf(out, "\nActive claims: %s\n", strings.Join(claimParts, ", "))
			}

			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to post as")
	cmd.Flags().StringP("reply-to", "r", "", "reply to message GUID (threading)")
	cmd.Flags().String("thread", "", "post in thread (guid, name, or path)")
	cmd.Flags().String("answer", "", "answer a question by guid or text")
	cmd.Flags().StringP("quote", "q", "", "quote message GUID (inline quote)")
	cmd.Flags().BoolP("silent", "s", false, "suppress output including unread mentions")

	_ = cmd.MarkFlagRequired("as")

	return cmd
}
