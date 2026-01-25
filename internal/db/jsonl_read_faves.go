package db

import (
	"encoding/json"
	"path/filepath"
)

// ReadFaves reads fave events from agents.jsonl for rebuilding the database.
func ReadFaves(projectPath string) ([]FaveEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readFavesMerged(projectPath)
	}
	return readFavesLegacy(projectPath)
}

func readFavesLegacy(projectPath string) ([]FaveEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	var events []FaveEvent
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "agent_fave":
			var record AgentFaveJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, FaveEvent{
				Type:     record.Type,
				AgentID:  record.AgentID,
				ItemType: record.ItemType,
				ItemGUID: record.ItemGUID,
				FavedAt:  record.FavedAt,
			})
		case "agent_unfave":
			var record AgentUnfaveJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, FaveEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				ItemType:  record.ItemType,
				ItemGUID:  record.ItemGUID,
				UnfavedAt: record.UnfavedAt,
			})
		case "fave_remove":
			var record struct {
				AgentID  string `json:"agent_id"`
				ItemType string `json:"item_type"`
				ItemGUID string `json:"item_guid"`
				TS       int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, FaveEvent{
				Type:      "fave_remove",
				AgentID:   record.AgentID,
				ItemType:  record.ItemType,
				ItemGUID:  record.ItemGUID,
				UnfavedAt: record.TS,
			})
		}
	}
	return events, nil
}

func readFavesMerged(projectPath string) ([]FaveEvent, error) {
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
		if typ != "agent_fave" && typ != "agent_unfave" && typ != "fave_remove" {
			continue
		}
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
	sortOrderedEvents(events)

	result := make([]FaveEvent, 0, len(events))
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "agent_fave":
			var record AgentFaveJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, FaveEvent{
				Type:     record.Type,
				AgentID:  record.AgentID,
				ItemType: record.ItemType,
				ItemGUID: record.ItemGUID,
				FavedAt:  record.FavedAt,
			})
		case "agent_unfave":
			var record AgentUnfaveJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, FaveEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				ItemType:  record.ItemType,
				ItemGUID:  record.ItemGUID,
				UnfavedAt: record.UnfavedAt,
			})
		case "fave_remove":
			var record struct {
				AgentID  string `json:"agent_id"`
				ItemType string `json:"item_type"`
				ItemGUID string `json:"item_guid"`
				TS       int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, FaveEvent{
				Type:      "fave_remove",
				AgentID:   record.AgentID,
				ItemType:  record.ItemType,
				ItemGUID:  record.ItemGUID,
				UnfavedAt: record.TS,
			})
		}
	}
	return result, nil
}

type roleEvent struct {
	Type       string
	AgentID    string
	RoleName   string
	SessionID  *string
	AssignedAt int64
	DroppedAt  int64
	StartedAt  int64
	StoppedAt  int64
}
