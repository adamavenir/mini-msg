package hooks

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewHookPrecompactCmd handles Claude PreCompact hooks.
func NewHookPrecompactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-precompact",
		Short: "PreCompact hook handler (internal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			output := hookOutput{}

			agentID := os.Getenv("FRAY_AGENT_ID")
			if agentID == "" {
				agentID = "<you>"
			}

			output.AdditionalContext = buildPrecompactContext(agentID)
			return writeHookOutput(cmd, output)
		},
	}

	return cmd
}

func buildPrecompactContext(agentID string) string {
	return fmt.Sprintf(`[fray] Context compacting. Preserve your work:
1. fray post %s/notes "# Handoff ..." --as %s
2. bd close <completed-issues>
3. fray bye %s

Or run /land for full checklist.`, agentID, agentID, agentID)
}
