package db

import (
	"encoding/json"
	"path/filepath"
)

// ReadReactions reads all reaction records from messages.jsonl.
func ReadReactions(projectPath string) ([]ReactionJSONLRecord, error) {
	if IsMultiMachineMode(projectPath) {
		return readReactionsMerged(projectPath)
	}
	return readReactionsLegacy(projectPath)
}

func readReactionsLegacy(projectPath string) ([]ReactionJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, messagesFile))
	if err != nil {
		return nil, err
	}

	var reactions []ReactionJSONLRecord
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		if envelope.Type == "reaction" {
			var record ReactionJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			reactions = append(reactions, record)
		}
	}
	return reactions, nil
}

func readReactionsMerged(projectPath string) ([]ReactionJSONLRecord, error) {
	lines, err := readSharedJSONLLines(projectPath, messagesFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil || typ != "reaction" {
			continue
		}
		seq := parseSeq(raw, int64(entry.Index))
		ts := messageEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}
	sortOrderedEvents(events)

	reactions := make([]ReactionJSONLRecord, 0, len(events))
	for _, event := range events {
		var record ReactionJSONLRecord
		if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
			continue
		}
		reactions = append(reactions, record)
	}
	return reactions, nil
}

// FaveEvent represents a fave or unfave event for rebuilding.
type FaveEvent struct {
	Type      string
	AgentID   string
	ItemType  string
	ItemGUID  string
	FavedAt   int64
	UnfavedAt int64
}
