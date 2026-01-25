package db

import (
	"encoding/json"
	"path/filepath"
	"sort"
)

// ReadAgentDescriptors reads agent descriptor events from JSONL.
func ReadAgentDescriptors(projectPath string) ([]AgentDescriptor, error) {
	if IsMultiMachineMode(projectPath) {
		return readAgentDescriptorsMerged(projectPath)
	}
	return readAgentDescriptorsLegacy(projectPath)
}

func readAgentDescriptorsLegacy(projectPath string) ([]AgentDescriptor, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	descriptorMap := make(map[string]AgentDescriptor)
	for _, line := range lines {
		raw, typ := parseRawEnvelope(line)
		if raw == nil || typ != "agent_descriptor" {
			continue
		}
		var record AgentDescriptorJSONLRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record.AgentID == "" {
			continue
		}
		descriptorMap[record.AgentID] = AgentDescriptor{
			AgentID:      record.AgentID,
			DisplayName:  record.DisplayName,
			Capabilities: record.Capabilities,
			TS:           record.TS,
		}
	}

	return flattenAgentDescriptors(descriptorMap), nil
}

func readAgentDescriptorsMerged(projectPath string) ([]AgentDescriptor, error) {
	lines, err := readSharedJSONLLines(projectPath, agentStateFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil || typ != "agent_descriptor" {
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

	descriptorMap := make(map[string]AgentDescriptor)
	for _, event := range events {
		var record AgentDescriptorJSONLRecord
		if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
			continue
		}
		if record.AgentID == "" {
			continue
		}
		descriptorMap[record.AgentID] = AgentDescriptor{
			AgentID:      record.AgentID,
			DisplayName:  record.DisplayName,
			Capabilities: record.Capabilities,
			TS:           record.TS,
		}
	}

	return flattenAgentDescriptors(descriptorMap), nil
}

func flattenAgentDescriptors(descriptors map[string]AgentDescriptor) []AgentDescriptor {
	if len(descriptors) == 0 {
		return nil
	}
	ids := make([]string, 0, len(descriptors))
	for id := range descriptors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]AgentDescriptor, 0, len(ids))
	for _, id := range ids {
		result = append(result, descriptors[id])
	}
	return result
}
