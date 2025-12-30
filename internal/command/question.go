package command

import (
	"encoding/json"
	"fmt"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewQuestionCmd creates the question command.
func NewQuestionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "question <id>",
		Short: "View a question",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			question, err := resolveQuestionRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(question)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Question %s\n", question.GUID)
			fmt.Fprintf(out, "  status: %s\n", question.Status)
			fmt.Fprintf(out, "  from: @%s\n", question.FromAgent)
			if question.ToAgent != nil {
				fmt.Fprintf(out, "  to: @%s\n", *question.ToAgent)
			}
			if question.ThreadGUID != nil {
				thread, _ := db.GetThread(ctx.DB, *question.ThreadGUID)
				if thread != nil {
					path, _ := buildThreadPath(ctx.DB, thread)
					fmt.Fprintf(out, "  thread: %s (%s)\n", path, thread.GUID)
				} else {
					fmt.Fprintf(out, "  thread: %s\n", *question.ThreadGUID)
				}
			}
			if question.AskedIn != nil {
				fmt.Fprintf(out, "  asked_in: %s\n", *question.AskedIn)
			}
			if question.AnsweredIn != nil {
				fmt.Fprintf(out, "  answered_in: %s\n", *question.AnsweredIn)
			}
			fmt.Fprintf(out, "  re: %s\n", question.Re)
			return nil
		},
	}

	cmd.AddCommand(NewQuestionCloseCmd())

	return cmd
}

// NewQuestionCloseCmd creates the question close command.
func NewQuestionCloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <id>",
		Short: "Close a question",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			question, err := resolveQuestionRef(ctx.DB, args[0])
			if err != nil {
				return writeCommandError(cmd, err)
			}

			statusValue := string(types.QuestionStatusClosed)
			updated, err := db.UpdateQuestion(ctx.DB, question.GUID, db.QuestionUpdates{
				Status: types.OptionalString{Set: true, Value: &statusValue},
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.AppendQuestionUpdate(ctx.Project.DBPath, db.QuestionUpdateJSONLRecord{
				GUID:   updated.GUID,
				Status: &statusValue,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(updated)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Closed question %s\n", updated.GUID)
			return nil
		},
	}

	return cmd
}
