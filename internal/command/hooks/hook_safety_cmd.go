package hooks

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewHookSafetyCmd runs the safety guard check (for testing).
func NewHookSafetyCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hook-safety",
		Short:  "Run fray safety guard (internal)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// This command exists for testing; actual guard runs via Python
			fmt.Fprintln(cmd.OutOrStdout(), "Safety guard is implemented in Python.")
			fmt.Fprintln(cmd.OutOrStdout(), "Install with: fray hook-install --safety")
			return nil
		},
	}
}
