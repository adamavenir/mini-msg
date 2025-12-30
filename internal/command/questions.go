package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewQuestionsCmd creates the questions list command.
func NewQuestionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "questions [@agent]",
		Short: "List questions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			unasked, _ := cmd.Flags().GetBool("unasked")
			answered, _ := cmd.Flags().GetBool("answered")
			all, _ := cmd.Flags().GetBool("all")
			room, _ := cmd.Flags().GetBool("room")
			threadRef, _ := cmd.Flags().GetString("thread")

			if room && threadRef != "" {
				return writeCommandError(cmd, fmt.Errorf("--room cannot be combined with --thread"))
			}

			statuses := make([]types.QuestionStatus, 0)
			if all {
				statuses = nil
			} else {
				if unasked {
					statuses = append(statuses, types.QuestionStatusUnasked)
				}
				if answered {
					statuses = append(statuses, types.QuestionStatusAnswered)
				}
				if !unasked && !answered {
					statuses = append(statuses, types.QuestionStatusOpen)
				}
			}

			options := types.QuestionQueryOptions{
				Statuses: statuses,
			}
			if room {
				options.RoomOnly = true
			}

			if threadRef != "" {
				thread, err := resolveThreadRef(ctx.DB, threadRef)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				options.ThreadGUID = &thread.GUID
			} else if !all {
				options.RoomOnly = true
			}

			if len(args) == 1 {
				agentRef := strings.TrimPrefix(args[0], "@")
				resolved := ResolveAgentRef(agentRef, ctx.ProjectConfig)
				options.ToAgent = &resolved
			}

			questions, err := db.GetQuestions(ctx.DB, &options)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(questions)
			}

			out := cmd.OutOrStdout()
			if len(questions) == 0 {
				fmt.Fprintln(out, "No questions found")
				return nil
			}

			fmt.Fprintln(out, "Questions:")
			for _, question := range questions {
				threadLabel := "room"
				if question.ThreadGUID != nil {
					thread, _ := db.GetThread(ctx.DB, *question.ThreadGUID)
					if thread != nil {
						if path, err := buildThreadPath(ctx.DB, thread); err == nil && path != "" {
							threadLabel = path
						} else {
							threadLabel = thread.GUID
						}
					} else {
						threadLabel = *question.ThreadGUID
					}
				}
				toAgent := "--"
				if question.ToAgent != nil {
					toAgent = "@" + *question.ToAgent
				}
				fmt.Fprintf(out, "  [%s] %s @%s → %s (%s)\n", question.GUID, question.Status, question.FromAgent, toAgent, threadLabel)
				fmt.Fprintf(out, "    %s\n", question.Re)
			}
			return nil
		},
	}

	cmd.Flags().Bool("unasked", false, "show unasked questions")
	cmd.Flags().Bool("answered", false, "show answered questions")
	cmd.Flags().Bool("all", false, "show all questions")
	cmd.Flags().Bool("room", false, "show room-level questions only")
	cmd.Flags().String("thread", "", "filter by thread")

	return cmd
}
