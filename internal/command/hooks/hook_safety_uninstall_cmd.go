package hooks

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewHookUninstallCmd removes installed hooks.
func NewHookUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-uninstall",
		Short: "Remove fray Claude Code hooks",
		Long: `Remove fray Claude Code hooks.

By default, removes integration hooks (SessionStart, UserPromptSubmit, etc).
Use --safety to also/only remove safety guards.

Examples:
  fray hook-uninstall                   # Remove integration hooks
  fray hook-uninstall --safety          # Also remove safety guards
  fray hook-uninstall --safety --global # Remove safety guards from all projects
  fray hook-uninstall --precommit       # Also remove git pre-commit hook`,
		RunE: func(cmd *cobra.Command, args []string) error {
			safety, _ := cmd.Flags().GetBool("safety")
			global, _ := cmd.Flags().GetBool("global")
			precommit, _ := cmd.Flags().GetBool("precommit")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			projectDir := os.Getenv("CLAUDE_PROJECT_DIR")
			if projectDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				projectDir = cwd
			}

			out := cmd.OutOrStdout()

			// Handle --safety flag
			if safety {
				if err := uninstallSafetyHooks(projectDir, out, global); err != nil {
					return err
				}
				if global {
					fmt.Fprintln(out, "\nRestart Claude Code to apply changes.")
					return nil
				}
			}

			// Remove integration hooks (unless --global --safety only)
			if !global || !safety {
				if err := uninstallIntegrationHooks(projectDir, out, dryRun); err != nil {
					return err
				}
			}

			if precommit {
				uninstallPrecommitHook(projectDir, dryRun, out)
			}

			fmt.Fprintln(out, "\nRestart Claude Code to apply changes.")
			return nil
		},
	}

	cmd.Flags().Bool("safety", false, "remove safety guards")
	cmd.Flags().Bool("global", false, "remove from ~/.claude instead of .claude")
	cmd.Flags().Bool("precommit", false, "also remove git pre-commit hook")
	cmd.Flags().Bool("dry-run", false, "show what would be removed without removing")

	return cmd
}
