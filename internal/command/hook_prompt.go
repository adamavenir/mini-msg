package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewHookPromptCmd handles Claude UserPromptSubmit hooks.
func NewHookPromptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook-prompt",
		Short: "UserPromptSubmit hook handler (internal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			output := hookOutput{}

			agentID := os.Getenv("MM_AGENT_ID")
			if agentID == "" {
				return writeHookOutput(cmd, output)
			}

			projectPath := os.Getenv("CLAUDE_PROJECT_DIR")
			project, err := core.DiscoverProject(projectPath)
			if err != nil {
				return writeHookOutput(cmd, output)
			}

			dbConn, err := db.OpenDatabase(project)
			if err != nil {
				return writeHookOutput(cmd, output)
			}
			defer dbConn.Close()
			if err := db.InitSchema(dbConn); err != nil {
				return writeHookOutput(cmd, output)
			}

			roomMessages, mentionMessages, _ := fetchHookMessages(dbConn, agentID, 5, 3)
			if len(roomMessages) == 0 && len(mentionMessages) == 0 {
				return writeHookOutput(cmd, output)
			}

			output.AdditionalContext = buildHookPromptContext(agentID, roomMessages, mentionMessages)
			return writeHookOutput(cmd, output)
		},
	}

	return cmd
}

func buildHookPromptContext(agentID string, roomMessages, mentionMessages []types.Message) string {
	var parts []string

	if len(roomMessages) > 0 {
		last := roomMessages[len(roomMessages)-1]
		parts = append(parts, fmt.Sprintf("Room[%d]: latest [%s] %s", len(roomMessages), last.ID, last.FromAgent))
	}

	if len(mentionMessages) > 0 {
		parts = append(parts, fmt.Sprintf("@mentions[%d]", len(mentionMessages)))
		limit := len(mentionMessages)
		if limit > 2 {
			limit = 2
		}
		for i := 0; i < limit; i++ {
			msg := mentionMessages[i]
			preview := msg.Body
			if len(preview) > 60 {
				preview = preview[:60] + "..."
			}
			parts = append(parts, fmt.Sprintf("  [%s] %s: %s", msg.ID, msg.FromAgent, preview))
		}
	}

	context := fmt.Sprintf("[mm %s] %s (mm get %s for full view)", agentID, strings.Join(parts, " | "), agentID)
	return context
}
