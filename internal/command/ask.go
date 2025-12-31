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

// NewAskCmd creates the ask command.
func NewAskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ask <question|guid>",
		Short: "Ask a question",
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

			questionInput := strings.TrimSpace(args[0])
			var question *types.Question
			question, err = resolveQuestionRef(ctx.DB, questionInput)
			if err != nil && !strings.Contains(err.Error(), "not found") {
				return writeCommandError(cmd, err)
			}

			toRef, _ := cmd.Flags().GetString("to")
			var toAgent *string
			if toRef != "" {
				resolved := ResolveAgentRef(toRef, ctx.ProjectConfig)
				toAgent = &resolved
			}

			threadRef, _ := cmd.Flags().GetString("thread")
			var thread *types.Thread
			if threadRef != "" {
				thread, err = resolveThreadRef(ctx.DB, threadRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			if question != nil {
				if thread != nil && question.ThreadGUID != nil && *question.ThreadGUID != thread.GUID {
					return writeCommandError(cmd, fmt.Errorf("question already belongs to another thread"))
				}
			}

			now := time.Now().Unix()
			if question == nil {
				threadGUID := (*string)(nil)
				if thread != nil {
					threadGUID = &thread.GUID
				}
				createdQuestion, err := db.CreateQuestion(ctx.DB, types.Question{
					Re:         questionInput,
					FromAgent:  agentID,
					ToAgent:    toAgent,
					Status:     types.QuestionStatusOpen,
					ThreadGUID: threadGUID,
					CreatedAt:  now,
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendQuestion(ctx.Project.DBPath, createdQuestion); err != nil {
					return writeCommandError(cmd, err)
				}
				question = &createdQuestion
			}

			body := question.Re
			if toAgent != nil {
				body = fmt.Sprintf("@%s %s", *toAgent, question.Re)
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			mentions := core.ExtractMentions(body, bases)
			mentions = core.ExpandAllMention(mentions, bases)

			home := ""
			if thread != nil {
				home = thread.GUID
			} else if question.ThreadGUID != nil {
				home = *question.ThreadGUID
			}

			created, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: agentID,
				Body:      body,
				Mentions:  mentions,
				Home:      home,
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

			statusValue := string(types.QuestionStatusOpen)
			questionUpdates := db.QuestionUpdates{
				Status:  types.OptionalString{Set: true, Value: &statusValue},
				AskedIn: types.OptionalString{Set: true, Value: &created.ID},
			}
			if toAgent != nil {
				questionUpdates.ToAgent = types.OptionalString{Set: true, Value: toAgent}
			}
			if thread != nil && question.ThreadGUID == nil {
				questionUpdates.ThreadGUID = types.OptionalString{Set: true, Value: &thread.GUID}
			}

			updated, err := db.UpdateQuestion(ctx.DB, question.GUID, questionUpdates)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			updateRecord := db.QuestionUpdateJSONLRecord{
				GUID:    updated.GUID,
				Status:  &statusValue,
				AskedIn: &created.ID,
			}
			if toAgent != nil {
				updateRecord.ToAgent = toAgent
			}
			if thread != nil && question.ThreadGUID == nil {
				updateRecord.ThreadGUID = &thread.GUID
			}
			if err := db.AppendQuestionUpdate(ctx.Project.DBPath, updateRecord); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"question": updated,
					"message":  created,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Asked %s (message %s)\n", updated.GUID, created.ID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to ask as")
	cmd.Flags().String("to", "", "agent to ask")
	cmd.Flags().String("thread", "", "thread guid or path")
	_ = cmd.MarkFlagRequired("as")

	return cmd
}
