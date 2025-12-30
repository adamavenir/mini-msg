package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewWonderCmd creates the wonder command.
func NewWonderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wonder <question>",
		Short: "Create an unasked question",
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

			now := time.Now().Unix()
			question, err := db.CreateQuestion(ctx.DB, types.Question{
				Re:        args[0],
				FromAgent: agentID,
				Status:    types.QuestionStatusUnasked,
				CreatedAt: now,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendQuestion(ctx.Project.DBPath, question); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(question)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created question %s\n", question.GUID)
			return nil
		},
	}

	cmd.Flags().String("as", "", "agent ID to create the question as")
	_ = cmd.MarkFlagRequired("as")

	return cmd
}
