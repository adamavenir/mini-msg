package command

import (
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// NewViewCmd creates the view command.
func NewViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "View full message without truncation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			messageID := strings.TrimPrefix(strings.TrimSpace(args[0]), "#")
			messageID = strings.TrimPrefix(messageID, "@")

			msg, err := db.GetMessage(ctx.DB, messageID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if msg == nil {
				msg, err = db.GetMessageByPrefix(ctx.DB, messageID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}
			if msg == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Message %s not found\n", messageID)
				return nil
			}

			color := getAgentColor(msg.FromAgent, msg.Type, nil)
			if color != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%sMessage #%s from @%s:%s\n", color, msg.ID, msg.FromAgent, reset)
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s%s\n", color, msg.Body, reset)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Message #%s from @%s:\n", msg.ID, msg.FromAgent)
			fmt.Fprintln(cmd.OutOrStdout(), msg.Body)
			return nil
		},
	}

	return cmd
}
