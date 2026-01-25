package db

import (
	"encoding/json"
	"path/filepath"
)

// TriggerEvent represents a session trigger with its context.
type TriggerEvent struct {
	AgentID     string  `json:"agent_id"`
	SessionID   string  `json:"session_id"`
	TriggeredBy *string `json:"triggered_by,omitempty"`
	ThreadGUID  *string `json:"thread_guid,omitempty"`
	StartedAt   int64   `json:"started_at"`
	EndedAt     *int64  `json:"ended_at,omitempty"`
	ExitCode    *int    `json:"exit_code,omitempty"`
	DurationMs  *int64  `json:"duration_ms,omitempty"`
}

// PresenceEvent represents a presence state change for audit.
type PresenceEvent struct {
	AgentID string  `json:"agent_id"`
	From    string  `json:"from"`
	To      string  `json:"to"`
	Status  *string `json:"status,omitempty"`
	Reason  string  `json:"reason"`
	Source  string  `json:"source"`
	TS      int64   `json:"ts"`
}

// ReadTriggerEvents reads session start/end events from agents.jsonl.
// Returns events sorted by timestamp descending (most recent first).
// Deduplicates by session ID - each session appears once with its latest trigger info.
func ReadTriggerEvents(projectPath string) ([]TriggerEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readTriggerEventsRuntime(projectPath)
	}
	return readTriggerEventsLegacy(projectPath)
}

func readTriggerEventsLegacy(projectPath string) ([]TriggerEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	// Map session_id -> TriggerEvent for merging start/end
	sessionMap := make(map[string]*TriggerEvent)
	sessionSeen := make(map[string]bool)
	var sessionOrder []string

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "session_start":
			var record SessionStartJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			event := &TriggerEvent{
				AgentID:     record.AgentID,
				SessionID:   record.SessionID,
				TriggeredBy: record.TriggeredBy,
				ThreadGUID:  record.ThreadGUID,
				StartedAt:   record.StartedAt,
			}
			sessionMap[record.SessionID] = event
			// Only add to order once (first occurrence)
			if !sessionSeen[record.SessionID] {
				sessionSeen[record.SessionID] = true
				sessionOrder = append(sessionOrder, record.SessionID)
			}

		case "session_end":
			var record SessionEndJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if event, ok := sessionMap[record.SessionID]; ok {
				event.EndedAt = &record.EndedAt
				event.ExitCode = &record.ExitCode
				event.DurationMs = &record.DurationMs
			}
		}
	}

	// Build result in reverse order (most recent first)
	events := make([]TriggerEvent, 0, len(sessionOrder))
	for i := len(sessionOrder) - 1; i >= 0; i-- {
		if event, ok := sessionMap[sessionOrder[i]]; ok {
			events = append(events, *event)
		}
	}
	return events, nil
}

func readTriggerEventsRuntime(projectPath string) ([]TriggerEvent, error) {
	lines, err := readLocalRuntimeLines(projectPath)
	if err != nil {
		return nil, err
	}

	sessionMap := make(map[string]*TriggerEvent)
	sessionSeen := make(map[string]bool)
	var sessionOrder []string

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "session_start":
			var record SessionStartJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			event := &TriggerEvent{
				AgentID:     record.AgentID,
				SessionID:   record.SessionID,
				TriggeredBy: record.TriggeredBy,
				ThreadGUID:  record.ThreadGUID,
				StartedAt:   record.StartedAt,
			}
			sessionMap[record.SessionID] = event
			if !sessionSeen[record.SessionID] {
				sessionSeen[record.SessionID] = true
				sessionOrder = append(sessionOrder, record.SessionID)
			}
		case "session_end":
			var record SessionEndJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if event, ok := sessionMap[record.SessionID]; ok {
				event.EndedAt = &record.EndedAt
				event.ExitCode = &record.ExitCode
				event.DurationMs = &record.DurationMs
			}
		}
	}

	events := make([]TriggerEvent, 0, len(sessionOrder))
	for i := len(sessionOrder) - 1; i >= 0; i-- {
		if event, ok := sessionMap[sessionOrder[i]]; ok {
			events = append(events, *event)
		}
	}
	return events, nil
}

// ReadPresenceEvents reads all presence_event records from agents.jsonl.
// Returns events sorted by timestamp descending (most recent first).
func ReadPresenceEvents(projectPath string) ([]PresenceEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readPresenceEventsRuntime(projectPath)
	}
	return readPresenceEventsLegacy(projectPath)
}

func readPresenceEventsLegacy(projectPath string) ([]PresenceEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	var events []PresenceEvent
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		if envelope.Type == "presence_event" {
			var record PresenceEventJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, PresenceEvent{
				AgentID: record.AgentID,
				From:    record.From,
				To:      record.To,
				Status:  record.Status,
				Reason:  record.Reason,
				Source:  record.Source,
				TS:      record.TS,
			})
		}
	}

	// Reverse to get most recent first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func readPresenceEventsRuntime(projectPath string) ([]PresenceEvent, error) {
	lines, err := readLocalRuntimeLines(projectPath)
	if err != nil {
		return nil, err
	}

	var events []PresenceEvent
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		if envelope.Type == "presence_event" {
			var record PresenceEventJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, PresenceEvent{
				AgentID: record.AgentID,
				From:    record.From,
				To:      record.To,
				Status:  record.Status,
				Reason:  record.Reason,
				Source:  record.Source,
				TS:      record.TS,
			})
		}
	}

	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}
