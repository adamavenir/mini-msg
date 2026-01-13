// Package usage provides token usage parsing for AI coding assistants.
// Supports Claude Code and Codex transcript formats.
package usage

// SessionUsage represents aggregated token usage for a session.
type SessionUsage struct {
	SessionID      string `json:"session_id"`
	Driver         string `json:"driver"` // "claude" or "codex"
	Model          string `json:"model"`
	InputTokens    int64  `json:"input_tokens"`
	OutputTokens   int64  `json:"output_tokens"`
	CachedTokens   int64  `json:"cached_tokens"`
	ContextLimit   int64  `json:"context_limit"`
	ContextPercent int    `json:"context_percent"`
}

// GetSessionUsage fetches token usage for a session ID.
// Searches both Claude Code and Codex directories.
func GetSessionUsage(sessionID string) (*SessionUsage, error) {
	if sessionID == "" {
		return nil, nil
	}

	// Try Claude Code first
	if path := findClaudeTranscript(sessionID); path != "" {
		return parseClaudeTranscript(sessionID, path)
	}

	// Try Codex
	if path := findCodexTranscript(sessionID); path != "" {
		return parseCodexTranscript(sessionID, path)
	}

	// Not found - return empty usage
	return &SessionUsage{
		SessionID:    sessionID,
		ContextLimit: 200000,
	}, nil
}

// GetSessionUsageByDriver fetches token usage for a session ID using a specific driver.
func GetSessionUsageByDriver(sessionID, driver string) (*SessionUsage, error) {
	if sessionID == "" {
		return nil, nil
	}

	switch driver {
	case "claude":
		if path := findClaudeTranscript(sessionID); path != "" {
			return parseClaudeTranscript(sessionID, path)
		}
	case "codex":
		if path := findCodexTranscript(sessionID); path != "" {
			return parseCodexTranscript(sessionID, path)
		}
	default:
		// Unknown driver, try both
		return GetSessionUsage(sessionID)
	}

	return &SessionUsage{
		SessionID:    sessionID,
		Driver:       driver,
		ContextLimit: getDefaultContextLimit(driver),
	}, nil
}

func getDefaultContextLimit(driver string) int64 {
	switch driver {
	case "codex":
		return 128000 // GPT-5 context
	default:
		return 200000 // Claude default
	}
}
