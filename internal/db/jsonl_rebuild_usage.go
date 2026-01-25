package db

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/adamavenir/fray/internal/types"
)

// GetLatestUsageSnapshot reads the most recent usage snapshot for a session from JSONL.
// Returns nil if no snapshot exists for the session.
func GetLatestUsageSnapshot(projectPath, sessionID string) *types.UsageSnapshot {
	frayDir := resolveFrayDir(projectPath)
	agentsPath := filepath.Join(frayDir, agentsFile)

	file, err := os.Open(agentsPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var latest *types.UsageSnapshot
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			continue
		}

		recordType, ok := raw["type"].(string)
		if !ok || recordType != "usage_snapshot" {
			continue
		}

		sid, _ := raw["session_id"].(string)
		if sid != sessionID {
			continue
		}

		// Decode as snapshot
		snapshot := &types.UsageSnapshot{
			SessionID: sid,
		}
		if v, ok := raw["agent_id"].(string); ok {
			snapshot.AgentID = v
		}
		if v, ok := raw["driver"].(string); ok {
			snapshot.Driver = v
		}
		if v, ok := raw["model"].(string); ok {
			snapshot.Model = v
		}
		if v, ok := raw["input_tokens"].(float64); ok {
			snapshot.InputTokens = int64(v)
		}
		if v, ok := raw["output_tokens"].(float64); ok {
			snapshot.OutputTokens = int64(v)
		}
		if v, ok := raw["cached_tokens"].(float64); ok {
			snapshot.CachedTokens = int64(v)
		}
		if v, ok := raw["context_limit"].(float64); ok {
			snapshot.ContextLimit = int64(v)
		}
		if v, ok := raw["context_percent"].(float64); ok {
			snapshot.ContextPercent = int(v)
		}
		if v, ok := raw["captured_at"].(float64); ok {
			snapshot.CapturedAt = int64(v)
		}

		// Keep the most recent (last in file wins)
		latest = snapshot
	}

	return latest
}

// GetAgentUsageSnapshots reads all usage snapshots for an agent from JSONL.
// Returns snapshots ordered by captured_at (oldest first).
func GetAgentUsageSnapshots(projectPath, agentID string, limit int) []types.UsageSnapshot {
	frayDir := resolveFrayDir(projectPath)
	agentsPath := filepath.Join(frayDir, agentsFile)

	file, err := os.Open(agentsPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var snapshots []types.UsageSnapshot
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			continue
		}

		recordType, ok := raw["type"].(string)
		if !ok || recordType != "usage_snapshot" {
			continue
		}

		aid, _ := raw["agent_id"].(string)
		if aid != agentID {
			continue
		}

		snapshot := types.UsageSnapshot{
			AgentID: aid,
		}
		if v, ok := raw["session_id"].(string); ok {
			snapshot.SessionID = v
		}
		if v, ok := raw["driver"].(string); ok {
			snapshot.Driver = v
		}
		if v, ok := raw["model"].(string); ok {
			snapshot.Model = v
		}
		if v, ok := raw["input_tokens"].(float64); ok {
			snapshot.InputTokens = int64(v)
		}
		if v, ok := raw["output_tokens"].(float64); ok {
			snapshot.OutputTokens = int64(v)
		}
		if v, ok := raw["cached_tokens"].(float64); ok {
			snapshot.CachedTokens = int64(v)
		}
		if v, ok := raw["context_limit"].(float64); ok {
			snapshot.ContextLimit = int64(v)
		}
		if v, ok := raw["context_percent"].(float64); ok {
			snapshot.ContextPercent = int(v)
		}
		if v, ok := raw["captured_at"].(float64); ok {
			snapshot.CapturedAt = int64(v)
		}

		snapshots = append(snapshots, snapshot)
	}

	// Return only the last N if limit is specified
	if limit > 0 && len(snapshots) > limit {
		return snapshots[len(snapshots)-limit:]
	}
	return snapshots
}
