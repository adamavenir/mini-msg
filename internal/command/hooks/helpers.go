package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type hookOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
	Continue          *bool  `json:"continue,omitempty"`
}

func writeHookOutput(cmd *cobra.Command, output hookOutput) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	return encoder.Encode(output)
}

func writeCommandError(cmd *cobra.Command, err error) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", err.Error())
	return err
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// WriteClaudeEnv writes FRAY_AGENT_ID to CLAUDE_ENV_FILE if set.
func WriteClaudeEnv(agentID string) bool {
	path := os.Getenv("CLAUDE_ENV_FILE")
	if path == "" {
		return false
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false
	}
	defer file.Close()

	if _, err := file.WriteString("FRAY_AGENT_ID=" + agentID + "\n"); err != nil {
		return false
	}
	return true
}
