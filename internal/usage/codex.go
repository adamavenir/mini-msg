package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// codexEvent represents a single event from Codex's JSONL transcript
type codexEvent struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// codexTokenPayload represents the token_count payload structure
type codexTokenPayload struct {
	Type string `json:"type"`
	Info *struct {
		Model              string `json:"model"`
		ModelName          string `json:"model_name"`
		ModelContextWindow int64  `json:"model_context_window"`
		TotalTokenUsage    *struct {
			InputTokens           int64 `json:"input_tokens"`
			CachedInputTokens     int64 `json:"cached_input_tokens"`
			CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
			OutputTokens          int64 `json:"output_tokens"`
			ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
			TotalTokens           int64 `json:"total_tokens"`
		} `json:"total_token_usage"`
		LastTokenUsage *struct {
			InputTokens           int64 `json:"input_tokens"`
			CachedInputTokens     int64 `json:"cached_input_tokens"`
			CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
			OutputTokens          int64 `json:"output_tokens"`
			ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
			TotalTokens           int64 `json:"total_tokens"`
		} `json:"last_token_usage"`
		Metadata *struct {
			Model string `json:"model"`
		} `json:"metadata"`
	} `json:"info"`
}

// codexTurnContextPayload represents turn_context events that contain model info
type codexTurnContextPayload struct {
	Model string `json:"model"`
}

// getCodexPaths returns directories where Codex stores sessions
func getCodexPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	paths := []string{}

	// Check CODEX_HOME env var
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		sessionsPath := filepath.Join(codexHome, "sessions")
		if info, err := os.Stat(sessionsPath); err == nil && info.IsDir() {
			paths = append(paths, sessionsPath)
		}
	}

	// Default path (~/.codex/sessions)
	defaultPath := filepath.Join(home, ".codex", "sessions")
	if info, err := os.Stat(defaultPath); err == nil && info.IsDir() {
		// Avoid duplicates
		found := false
		for _, p := range paths {
			if p == defaultPath {
				found = true
				break
			}
		}
		if !found {
			paths = append(paths, defaultPath)
		}
	}

	return paths
}

// findCodexTranscript searches Codex directories for a session's transcript.
// Codex filenames are: rollout-YYYY-MM-DDTHH-MM-SS-{session-id}.jsonl
// where session-id is a UUID like 3e37c2ff-c596-4cab-8346-a5d4e6b81514
func findCodexTranscript(sessionID string) string {
	for _, basePath := range getCodexPaths() {
		var foundPath string

		// Codex sessions are in date subdirectories: sessions/YYYY/MM/DD/
		filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // Skip errors
			}
			if d.IsDir() {
				return nil
			}

			// Check if this file contains the session ID
			// Filenames are: rollout-YYYY-MM-DDTHH-MM-SS-{session-id}.jsonl
			fileName := filepath.Base(path)
			if strings.Contains(fileName, sessionID) && strings.HasSuffix(fileName, ".jsonl") {
				foundPath = path
				return filepath.SkipAll // Stop walking
			}

			return nil
		})

		if foundPath != "" {
			return foundPath
		}

		// Also check flat structure (fallback for older format)
		transcriptPath := filepath.Join(basePath, sessionID+".jsonl")
		if _, err := os.Stat(transcriptPath); err == nil {
			return transcriptPath
		}
	}

	return ""
}

// parseCodexTranscript reads a Codex transcript and aggregates token usage
func parseCodexTranscript(sessionID, transcriptPath string) (*SessionUsage, error) {
	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var inputTokens, outputTokens, cachedTokens int64
	var model string
	var contextWindow int64

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var event codexEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		// Check turn_context events for model info
		if event.Type == "turn_context" {
			var turnCtx codexTurnContextPayload
			if json.Unmarshal(event.Payload, &turnCtx) == nil && turnCtx.Model != "" {
				model = turnCtx.Model
			}
			continue
		}

		// Process event_msg types with token_count payloads
		if event.Type != "event_msg" {
			continue
		}

		var payload codexTokenPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}

		if payload.Type != "token_count" {
			continue
		}

		if payload.Info == nil {
			continue
		}

		// Extract model from token_count info
		if payload.Info.Model != "" {
			model = payload.Info.Model
		} else if payload.Info.ModelName != "" {
			model = payload.Info.ModelName
		} else if payload.Info.Metadata != nil && payload.Info.Metadata.Model != "" {
			model = payload.Info.Metadata.Model
		}

		// Use model_context_window from the transcript if available
		if payload.Info.ModelContextWindow > 0 {
			contextWindow = payload.Info.ModelContextWindow
		}

		// Use total_token_usage for cumulative counts (last entry will have final totals)
		if payload.Info.TotalTokenUsage != nil {
			usage := payload.Info.TotalTokenUsage
			inputTokens = usage.InputTokens
			outputTokens = usage.OutputTokens
			cachedTokens = usage.CachedInputTokens
			if cachedTokens == 0 {
				cachedTokens = usage.CacheReadInputTokens
			}
		}
	}

	// Use context window from transcript if available, otherwise fall back to model-based lookup
	contextLimit := contextWindow
	if contextLimit == 0 {
		contextLimit = getCodexContextLimit(model)
	}

	contextPercent := 0
	if contextLimit > 0 {
		contextPercent = int((inputTokens * 100) / contextLimit)
	}

	return &SessionUsage{
		SessionID:      sessionID,
		Driver:         "codex",
		Model:          model,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
		CachedTokens:   cachedTokens,
		ContextLimit:   contextLimit,
		ContextPercent: contextPercent,
	}, nil
}

// getCodexContextLimit returns the context window size for a Codex/OpenAI model
func getCodexContextLimit(model string) int64 {
	// GPT-5 models
	if strings.Contains(model, "gpt-5") {
		return 128000
	}
	// GPT-4 models
	if strings.Contains(model, "gpt-4") {
		if strings.Contains(model, "turbo") || strings.Contains(model, "128k") {
			return 128000
		}
		return 8192
	}
	// Default
	return 128000
}
