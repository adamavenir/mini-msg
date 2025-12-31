package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/adamavenir/fray/internal/chat"
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// NewChatCmd creates the chat command.
func NewChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat [channel]",
		Short: "Interactive chat mode",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonMode, _ := cmd.Flags().GetBool("json"); jsonMode {
				return writeCommandError(cmd, fmt.Errorf("--json not supported for interactive chat"))
			}

			last, _ := cmd.Flags().GetInt("last")
			hideEvents, _ := cmd.Flags().GetBool("hide-events")
			showEvents, _ := cmd.Flags().GetBool("show-events")
			showUpdatesFlag, _ := cmd.Flags().GetBool("show-updates")
			archived, _ := cmd.Flags().GetBool("archived")
			force, _ := cmd.Flags().GetBool("force")

			var ctx *CommandContext
			var err error
			channelArg := ""
			if len(args) > 0 {
				channelArg = args[0]
			}

			if channelArg != "" {
				resolved, resolveErr := ResolveChannelContext(channelArg, "")
				if resolveErr != nil {
					return writeCommandError(cmd, resolveErr)
				}
				dbConn, err := db.OpenDatabase(resolved.Project)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.InitSchema(dbConn); err != nil {
					_ = dbConn.Close()
					return writeCommandError(cmd, err)
				}
				ctx = &CommandContext{
					DB:            dbConn,
					Project:       resolved.Project,
					ChannelID:     resolved.ChannelID,
					ChannelName:   resolved.ChannelName,
					ProjectConfig: resolved.ProjectConfig,
					Force:         force,
				}
			} else {
				ctx, err = GetContext(cmd)
				if err != nil {
					if shouldInitPrompt(err) {
						ctx, err = initForChat(cmd)
					}
					if err != nil {
						return writeCommandError(cmd, err)
					}
					if ctx == nil {
						return nil
					}
				}
			}

			defer ctx.DB.Close()

			username, err := db.GetConfig(ctx.DB, "username")
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if username == "" {
				username = promptUsername()
				if username == "" {
					return writeCommandError(cmd, fmt.Errorf("username is required"))
				}
				if err := db.SetConfig(ctx.DB, "username", username); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			showUpdates := true
			if hideEvents {
				showUpdates = false
			} else if showEvents {
				showUpdates = true
			} else if showUpdatesFlag {
				showUpdates = true
			}

			options := chat.Options{
				DB:              ctx.DB,
				ProjectName:     GetProjectName(ctx.Project.Root),
				ProjectRoot:     ctx.Project.Root,
				ProjectDBPath:   ctx.Project.DBPath,
				Username:        username,
				Last:            last,
				ShowUpdates:     showUpdates,
				IncludeArchived: archived,
			}

			return chat.Run(options)
		},
	}

	cmd.Flags().Int("last", 20, "show last N messages")
	cmd.Flags().Bool("hide-events", false, "hide event messages")
	cmd.Flags().Bool("show-events", false, "show event messages")
	cmd.Flags().Bool("show-updates", false, "include event messages (deprecated)")
	cmd.Flags().Bool("archived", false, "include archived messages")

	return cmd
}

func shouldInitPrompt(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not initialized") || strings.Contains(msg, "fray init") || strings.Contains(msg, "no channel context")
}

func initForChat(cmd *cobra.Command) (*CommandContext, error) {
	if !promptInit() {
		return nil, nil
	}

	project, err := core.InitProject("", false)
	if err != nil {
		return nil, err
	}
	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return nil, err
	}
	if err := db.InitSchema(dbConn); err != nil {
		_ = dbConn.Close()
		return nil, err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Initialized .fray/")

	return &CommandContext{DB: dbConn, Project: project}, nil
}

func promptUsername() string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stdout, "Enter your username: ")
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func promptInit() bool {
	if !isTTY(os.Stdin) {
		return false
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stdout, "Run `fray init`? [Y/n] ")
	text, _ := reader.ReadString('\n')
	trimmed := strings.ToLower(strings.TrimSpace(text))
	return trimmed == "" || trimmed == "y" || trimmed == "yes"
}
