package command

import "os"

// WriteClaudeEnv writes MM_AGENT_ID to CLAUDE_ENV_FILE if set.
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

	if _, err := file.WriteString("MM_AGENT_ID=" + agentID + "\n"); err != nil {
		return false
	}
	return true
}
