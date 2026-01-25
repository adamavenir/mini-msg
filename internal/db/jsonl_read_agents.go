package db

import (
	"encoding/json"
	"path/filepath"
)

// ReadAgents reads agent JSONL records and applies updates.
func ReadAgents(projectPath string) ([]AgentJSONLRecord, error) {
	if IsMultiMachineMode(projectPath) {
		return readAgentsRuntime(projectPath)
	}
	return readAgentsLegacy(projectPath)
}

func readAgentsLegacy(projectPath string) ([]AgentJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	agentMap := make(map[string]AgentJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "agent":
			var record AgentJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if _, ok := seen[record.AgentID]; !ok {
				seen[record.AgentID] = struct{}{}
				order = append(order, record.AgentID)
			}
			agentMap[record.AgentID] = record
		case "agent_update":
			var update AgentUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			existing, ok := agentMap[update.AgentID]
			if !ok {
				continue
			}
			if update.Status != nil {
				existing.Status = update.Status
			}
			if update.Purpose != nil {
				existing.Purpose = update.Purpose
			}
			if update.Avatar != nil {
				existing.Avatar = update.Avatar
			}
			if update.LastSeen != nil {
				existing.LastSeen = *update.LastSeen
			}
			if update.LeftAt != nil {
				existing.LeftAt = update.LeftAt
			}
			if update.Managed != nil {
				existing.Managed = *update.Managed
			}
			if update.Invoke != nil {
				existing.Invoke = update.Invoke
			}
			if update.Presence != nil {
				existing.Presence = *update.Presence
			}
			if update.MentionWatermark != nil {
				existing.MentionWatermark = update.MentionWatermark
			}
			if update.LastHeartbeat != nil {
				existing.LastHeartbeat = update.LastHeartbeat
			}
			if update.SessionMode != nil {
				existing.SessionMode = *update.SessionMode
			}
			if update.LastSessionID != nil {
				existing.LastSessionID = update.LastSessionID
			}
			agentMap[update.AgentID] = existing
		case "presence_event":
			var event PresenceEventJSONLRecord
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			existing, ok := agentMap[event.AgentID]
			if !ok {
				continue
			}
			existing.Presence = event.To
			agentMap[event.AgentID] = existing
			// session_start, session_end, session_heartbeat are events, not agent records
			// They are handled separately when needed
		}
	}

	agents := make([]AgentJSONLRecord, 0, len(order))
	for _, id := range order {
		record, ok := agentMap[id]
		if !ok {
			continue
		}
		agents = append(agents, record)
	}
	return agents, nil
}

func readAgentsRuntime(projectPath string) ([]AgentJSONLRecord, error) {
	lines, err := readLocalRuntimeLines(projectPath)
	if err != nil {
		return nil, err
	}

	agentMap := make(map[string]AgentJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "agent":
			var record AgentJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if _, ok := seen[record.AgentID]; !ok {
				seen[record.AgentID] = struct{}{}
				order = append(order, record.AgentID)
			}
			agentMap[record.AgentID] = record
		case "agent_update":
			var update AgentUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			existing, ok := agentMap[update.AgentID]
			if !ok {
				continue
			}
			if update.Status != nil {
				existing.Status = update.Status
			}
			if update.Purpose != nil {
				existing.Purpose = update.Purpose
			}
			if update.Avatar != nil {
				existing.Avatar = update.Avatar
			}
			if update.LastSeen != nil {
				existing.LastSeen = *update.LastSeen
			}
			if update.LeftAt != nil {
				existing.LeftAt = update.LeftAt
			}
			if update.Managed != nil {
				existing.Managed = *update.Managed
			}
			if update.Invoke != nil {
				existing.Invoke = update.Invoke
			}
			if update.Presence != nil {
				existing.Presence = *update.Presence
			}
			if update.MentionWatermark != nil {
				existing.MentionWatermark = update.MentionWatermark
			}
			if update.LastHeartbeat != nil {
				existing.LastHeartbeat = update.LastHeartbeat
			}
			if update.SessionMode != nil {
				existing.SessionMode = *update.SessionMode
			}
			if update.LastSessionID != nil {
				existing.LastSessionID = update.LastSessionID
			}
			agentMap[update.AgentID] = existing
		}
	}

	agents := make([]AgentJSONLRecord, 0, len(order))
	for _, id := range order {
		record, ok := agentMap[id]
		if !ok {
			continue
		}
		agents = append(agents, record)
	}
	return agents, nil
}
