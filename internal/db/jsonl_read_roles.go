package db

import (
	"encoding/json"
	"path/filepath"
)

// ReadRoles reads role events from agents.jsonl for rebuilding the database.
func ReadRoles(projectPath string) ([]roleEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readRolesMerged(projectPath)
	}
	return readRolesLegacy(projectPath)
}

func readRolesLegacy(projectPath string) ([]roleEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	var events []roleEvent
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "role_hold":
			var record RoleHoldJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:       record.Type,
				AgentID:    record.AgentID,
				RoleName:   record.RoleName,
				AssignedAt: record.AssignedAt,
			})
		case "role_drop":
			var record RoleDropJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				DroppedAt: record.DroppedAt,
			})
		case "role_play":
			var record RolePlayJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				SessionID: record.SessionID,
				StartedAt: record.StartedAt,
			})
		case "role_stop":
			var record RoleStopJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				StoppedAt: record.StoppedAt,
			})
		case "role_release":
			var record struct {
				AgentID  string `json:"agent_id"`
				RoleName string `json:"role_name"`
				TS       int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:      "role_release",
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				DroppedAt: record.TS,
			})
		}
	}
	return events, nil
}

func readRolesMerged(projectPath string) ([]roleEvent, error) {
	lines, err := readSharedJSONLLines(projectPath, agentStateFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil {
			continue
		}
		switch typ {
		case "role_hold", "role_drop", "role_play", "role_stop", "role_release":
			seq := parseSeq(raw, int64(entry.Index))
			ts := agentStateEventTimestamp(typ, raw)
			events = append(events, orderedJSONLEvent{
				Line:    entry.Line,
				Machine: entry.Machine,
				Seq:     seq,
				TS:      ts,
				Index:   entry.Index,
			})
		}
	}
	sortOrderedEvents(events)

	result := make([]roleEvent, 0, len(events))
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "role_hold":
			var record RoleHoldJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:       record.Type,
				AgentID:    record.AgentID,
				RoleName:   record.RoleName,
				AssignedAt: record.AssignedAt,
			})
		case "role_drop":
			var record RoleDropJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				DroppedAt: record.DroppedAt,
			})
		case "role_play":
			var record RolePlayJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				SessionID: record.SessionID,
				StartedAt: record.StartedAt,
			})
		case "role_stop":
			var record RoleStopJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				StoppedAt: record.StoppedAt,
			})
		case "role_release":
			var record struct {
				AgentID  string `json:"agent_id"`
				RoleName string `json:"role_name"`
				TS       int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:      "role_release",
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				DroppedAt: record.TS,
			})
		}
	}
	return result, nil
}
