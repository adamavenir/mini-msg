package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// claudeUsageEntry represents a single entry from Claude Code's JSONL transcript
type claudeUsageEntry struct {
	SessionID string `json:"sessionId"`
	Message   *struct {
		Usage *struct {
			InputTokens         int64 `json:"input_tokens"`
			OutputTokens        int64 `json:"output_tokens"`
			CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	} `json:"message"`
}

// getClaudePaths returns directories where Claude Code stores transcripts
func getClaudePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	paths := []string{}

	// XDG config path (~/.config/claude)
	xdgPath := filepath.Join(home, ".config", "claude", "projects")
	if info, err := os.Stat(xdgPath); err == nil && info.IsDir() {
		paths = append(paths, xdgPath)
	}

	// Legacy path (~/.claude)
	legacyPath := filepath.Join(home, ".claude", "projects")
	if info, err := os.Stat(legacyPath); err == nil && info.IsDir() {
		paths = append(paths, legacyPath)
	}

	return paths
}

// findClaudeTranscript searches Claude Code directories for a session's transcript
func findClaudeTranscript(sessionID string) string {
	for _, basePath := range getClaudePaths() {
		projectDirs, err := os.ReadDir(basePath)
		if err != nil {
			continue
		}

		for _, projectDir := range projectDirs {
			if !projectDir.IsDir() {
				continue
			}

			transcriptPath := filepath.Join(basePath, projectDir.Name(), sessionID+".jsonl")
			if _, err := os.Stat(transcriptPath); err == nil {
				return transcriptPath
			}
		}
	}

	return ""
}

// parseClaudeTranscript reads a Claude Code transcript and gets token usage.
// For context tracking, we use the LAST entry's input_tokens (current context size),
// not the sum of all entries (which would incorrectly accumulate).
func parseClaudeTranscript(sessionID, transcriptPath string) (*SessionUsage, error) {
	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lastInputTokens, totalOutputTokens, lastCachedTokens int64
	var model string

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		var entry claudeUsageEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Message != nil && entry.Message.Usage != nil {
			// Use LAST input tokens (represents current context size)
			lastInputTokens = entry.Message.Usage.InputTokens
			// Sum output tokens (cumulative generation)
			totalOutputTokens += entry.Message.Usage.OutputTokens
			// Use last cached tokens
			lastCachedTokens = entry.Message.Usage.CacheReadTokens
			if entry.Message.Model != "" {
				model = entry.Message.Model
			}
		}
	}

	contextLimit := getClaudeContextLimit(model)
	contextPercent := 0
	if contextLimit > 0 {
		contextPercent = int((lastInputTokens * 100) / contextLimit)
	}

	return &SessionUsage{
		SessionID:      sessionID,
		Driver:         "claude",
		Model:          model,
		InputTokens:    lastInputTokens,
		OutputTokens:   totalOutputTokens,
		CachedTokens:   lastCachedTokens,
		ContextLimit:   contextLimit,
		ContextPercent: contextPercent,
	}, nil
}

// getClaudeContextLimit returns the context window size for a Claude model
func getClaudeContextLimit(model string) int64 {
	// Models with 1M context
	if strings.Contains(model, "sonnet-1m") || strings.Contains(model, "1m") {
		return 1000000
	}
	// Default to 200k for most Claude models
	return 200000
}
