package command

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const AppName = "fray"

// Version is overwritten at build time using -ldflags.
var Version = "dev"

func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           AppName,
		Short:         "Fray - CLI for agent-to-agent messaging",
		Long:          "Fray is a lightweight agent-to-agent messaging CLI.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.Version = version
	cmd.SetVersionTemplate(AppName + " version {{.Version}}\n")
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	cmd.PersistentFlags().String("project", "", "operate in linked project")
	cmd.PersistentFlags().String("in", "", "operate in channel context")
	cmd.PersistentFlags().Bool("json", false, "output in JSON format")
	cmd.PersistentFlags().Bool("force", false, "force action (skip confirmations or suggestions)")

	cmd.AddCommand(
		NewInitCmd(),
		NewDestroyCmd(),
		NewNewCmd(),
		NewBatchUpdateCmd(),
		NewBackCmd(),
		NewByeCmd(),
		NewHereCmd(),
		NewWhoCmd(),
		NewWhoamiCmd(),
		NewNickCmd(),
		NewNicksCmd(),
		NewPostCmd(),
		NewEditCmd(),
		NewRmCmd(),
		NewClaimCmd(),
		NewClaimsCmd(),
		NewClearCmd(),
		NewStatusCmd(),
		NewGetCmd(),
		NewMentionsCmd(),
		NewQuickstartCmd(),
		NewHistoryCmd(),
		NewBetweenCmd(),
		NewReplyCmd(),
		NewThreadCmd(),
		NewThreadsCmd(),
		NewWonderCmd(),
		NewAskCmd(),
		NewQuestionsCmd(),
		NewQuestionCmd(),
		NewSurfaceCmd(),
		NewNoteCmd(),
		NewNotesCmd(),
		NewMetaCmd(),
		NewUnreactCmd(),
		NewChatCmd(),
		NewWatchCmd(),
		NewPruneCmd(),
		NewConfigCmd(),
		NewRosterCmd(),
		NewInfoCmd(),
		NewRenameCmd(),
		NewMergeCmd(),
		NewViewCmd(),
		NewVersionsCmd(),
		NewFilterCmd(),
		NewLsCmd(),
		NewMigrateCmd(),
		NewHookInstallCmd(),
		NewHookSessionCmd(),
		NewHookPromptCmd(),
		NewHookPrecommitCmd(),
	)

	return cmd
}

func Execute() error {
	os.Args = rewriteMentionArgs(os.Args)
	return NewRootCmd(Version).Execute()
}

func rewriteMentionArgs(args []string) []string {
	if len(args) < 2 {
		return args
	}
	idx := findFirstNonFlagArg(args[1:])
	if idx == -1 {
		return args
	}
	fullIdx := idx + 1
	if fullIdx >= len(args) {
		return args
	}
	if !strings.HasPrefix(args[fullIdx], "@") {
		return args
	}
	updated := make([]string, 0, len(args)+1)
	updated = append(updated, args[:fullIdx]...)
	updated = append(updated, "mentions")
	updated = append(updated, args[fullIdx:]...)
	return updated
}

func findFirstNonFlagArg(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				return i + 1
			}
			return -1
		}
		if strings.HasPrefix(arg, "--") {
			if strings.HasPrefix(arg, "--project=") || strings.HasPrefix(arg, "--in=") {
				continue
			}
			if arg == "--project" || arg == "--in" {
				i++
				continue
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return i
	}
	return -1
}
