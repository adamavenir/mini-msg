package command

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewWatchCmd creates the watch command.
func NewWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Stream messages in real-time",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			last, _ := cmd.Flags().GetInt("last")
			includeArchived, _ := cmd.Flags().GetBool("archived")

			projectName := GetProjectName(ctx.Project.Root)
			out := cmd.OutOrStdout()
			var agentBases map[string]struct{}
			if !ctx.JSONMode {
				agentBases, err = db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			}

			var cursor *types.MessageCursor
			if last == 0 {
				cursor, err = db.GetLastMessageCursor(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if !ctx.JSONMode {
					fmt.Fprintln(out, "--- watching (Ctrl+C to stop) ---")
				}
			} else {
				recent, err := db.GetMessages(ctx.DB, &types.MessageQueryOptions{Limit: last, IncludeArchived: includeArchived})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				recent, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, recent)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if len(recent) > 0 {
					if ctx.JSONMode {
						for _, msg := range recent {
							_ = json.NewEncoder(out).Encode(msg)
						}
					} else {
						for _, msg := range recent {
							fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
						}
						fmt.Fprintln(out, "--- watching (Ctrl+C to stop) ---")
					}
					lastMsg := recent[len(recent)-1]
					cursor = &types.MessageCursor{GUID: lastMsg.ID, TS: lastMsg.TS}
				} else if !ctx.JSONMode {
					fmt.Fprintln(out, "--- watching (Ctrl+C to stop) ---")
				}
			}

			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-stop:
					return nil
				case <-ticker.C:
					newMessages, err := db.GetMessages(ctx.DB, &types.MessageQueryOptions{Since: cursor, IncludeArchived: includeArchived})
					if err != nil {
						return writeCommandError(cmd, err)
					}
					newMessages, err = db.ApplyMessageEditCounts(ctx.Project.DBPath, newMessages)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					if len(newMessages) == 0 {
						continue
					}
					if ctx.JSONMode {
						encoder := json.NewEncoder(out)
						for _, msg := range newMessages {
							_ = encoder.Encode(msg)
						}
					} else {
						for _, msg := range newMessages {
							fmt.Fprintln(out, FormatMessage(msg, projectName, agentBases))
						}
					}
					lastMsg := newMessages[len(newMessages)-1]
					cursor = &types.MessageCursor{GUID: lastMsg.ID, TS: lastMsg.TS}
				}
			}
		},
	}

	cmd.Flags().Int("last", 10, "show last N messages before streaming")
	cmd.Flags().Bool("archived", false, "include archived messages")
	return cmd
}
