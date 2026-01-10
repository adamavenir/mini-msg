package command

import (
	"os"
	"strings"

	"github.com/adamavenir/fray/internal/command/hooks"
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
		NewAgentCmd(),
		NewDaemonCmd(),
		NewDashboardCmd(),
		NewBatchUpdateCmd(),
		NewBackCmd(),
		NewBRBCmd(),
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
		NewQuickstartCmd(),
		NewHowtoCmd(),
		NewReplyCmd(),
		NewThreadCmd(),
		NewThreadsCmd(),
		NewPinCmd(),
		NewUnpinCmd(),
		NewMvCmd(),
		NewFollowCmd(),
		NewUnfollowCmd(),
		NewMuteCmd(),
		NewUnmuteCmd(),
		NewAddCmd(),
		NewRemoveCmd(),
		NewArchiveCmd(),
		NewRestoreCmd(),
		NewAnchorCmd(),
		NewWonderCmd(),
		NewAskCmd(),
		NewQuestionsCmd(),
		NewQuestionCmd(),
		NewAnswerCmd(),
		NewSurfaceCmd(),
		NewReactCmd(),
		NewFaveCmd(),
		NewUnfaveCmd(),
		NewFavesCmd(),
		NewReactionsCmd(),
		NewChatCmd(),
		NewWatchCmd(),
		NewPruneCmd(),
		NewConfigCmd(),
		NewRosterCmd(),
		NewInfoCmd(),
		NewRenameCmd(),
		NewMergeCmd(),
		NewVersionsCmd(),
		NewFilterCmd(),
		NewLsCmd(),
		NewMigrateCmd(),
		NewRoleCmd(),
		NewRolesCmd(),
		NewRebuildCmd(),
		NewHeartbeatCmd(),
		NewClockCmd(),
		NewCursorCmd(),
		NewWakeCmd(),
		NewInstallNotifierCmd(),
		hooks.NewHookInstallCmd(),
		hooks.NewHookUninstallCmd(),
		hooks.NewHookSessionCmd(),
		hooks.NewHookPromptCmd(),
		hooks.NewHookPrecommitCmd(),
		hooks.NewHookPrecompactCmd(),
		hooks.NewHookSessionEndCmd(),
		hooks.NewHookStatuslineCmd(),
	)

	return cmd
}

func Execute() error {
	os.Args = rewriteMentionArgs(os.Args)
	os.Args = rewriteMessageIDArgs(os.Args)
	return NewRootCmd(Version).Execute()
}

// rewriteMessageIDArgs rewrites "fray msg-xxx" to "fray get msg-xxx".
func rewriteMessageIDArgs(args []string) []string {
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
	arg := args[fullIdx]
	// Check if it looks like a message ID (msg-xxxx or short prefix)
	if strings.HasPrefix(arg, "msg-") {
		updated := make([]string, 0, len(args)+1)
		updated = append(updated, args[:fullIdx]...)
		updated = append(updated, "get")
		updated = append(updated, args[fullIdx:]...)
		return updated
	}
	return args
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
	// Rewrite "@agent" to "get notifs --as agent"
	agent := strings.TrimPrefix(args[fullIdx], "@")
	updated := make([]string, 0, len(args)+3)
	updated = append(updated, args[:fullIdx]...)
	updated = append(updated, "get", "notifs", "--as", agent)
	updated = append(updated, args[fullIdx+1:]...)
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
