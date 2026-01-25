package db

import (
	"encoding/json"
	"path/filepath"
)

// ReadGhostCursors reads ghost cursor events from agents.jsonl for rebuilding the database.
// Ghost cursors track recommended read positions for session handoffs.
func ReadGhostCursors(projectPath string) ([]GhostCursorJSONLRecord, error) {
	if IsMultiMachineMode(projectPath) {
		return readGhostCursorsMerged(projectPath)
	}
	return readGhostCursorsLegacy(projectPath)
}

func readGhostCursorsLegacy(projectPath string) ([]GhostCursorJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	// Track latest cursor per (agent, home) pair
	type cursorKey struct {
		agentID string
		home    string
	}
	cursorMap := make(map[cursorKey]GhostCursorJSONLRecord)

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "ghost_cursor":
			var record GhostCursorJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			key := cursorKey{agentID: record.AgentID, home: record.Home}
			cursorMap[key] = record
		case "cursor_clear":
			var record struct {
				AgentID string `json:"agent_id"`
				Home    string `json:"home"`
			}
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if record.AgentID == "" || record.Home == "" {
				continue
			}
			key := cursorKey{agentID: record.AgentID, home: record.Home}
			delete(cursorMap, key)
		}
	}

	cursors := make([]GhostCursorJSONLRecord, 0, len(cursorMap))
	for _, cursor := range cursorMap {
		cursors = append(cursors, cursor)
	}
	return cursors, nil
}

func readGhostCursorsMerged(projectPath string) ([]GhostCursorJSONLRecord, error) {
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
		if typ != "ghost_cursor" && typ != "cursor_clear" {
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

	type cursorKey struct {
		agentID string
		home    string
	}
	cursorMap := make(map[cursorKey]GhostCursorJSONLRecord)

	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "ghost_cursor":
			var record GhostCursorJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			key := cursorKey{agentID: record.AgentID, home: record.Home}
			cursorMap[key] = record
		case "cursor_clear":
			var record struct {
				AgentID string `json:"agent_id"`
				Home    string `json:"home"`
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			if record.AgentID == "" || record.Home == "" {
				continue
			}
			key := cursorKey{agentID: record.AgentID, home: record.Home}
			delete(cursorMap, key)
		}
	}

	cursors := make([]GhostCursorJSONLRecord, 0, len(cursorMap))
	for _, cursor := range cursorMap {
		cursors = append(cursors, cursor)
	}
	return cursors, nil
}
