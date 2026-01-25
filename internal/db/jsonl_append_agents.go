package db

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// AppendAgent appends an agent record to JSONL.
func AppendAgent(projectPath string, agent types.Agent) error {
	config, err := ReadProjectConfig(projectPath)
	if err != nil {
		return err
	}

	channelName := ""
	channelID := ""
	if config != nil {
		channelName = config.ChannelName
		channelID = config.ChannelID
	}

	name := agent.AgentID
	globalName := name
	if channelName != "" {
		globalName = fmt.Sprintf("%s-%s", channelName, name)
	}

	createdAt := time.Unix(agent.RegisteredAt, 0).UTC().Format(time.RFC3339)
	activeStatus := "active"
	if agent.LeftAt != nil {
		activeStatus = "inactive"
	}

	record := AgentJSONLRecord{
		Type:             "agent",
		ID:               agent.GUID,
		Name:             name,
		GlobalName:       &globalName,
		HomeChannel:      nil,
		CreatedAt:        &createdAt,
		ActiveStatus:     &activeStatus,
		AgentID:          agent.AgentID,
		Status:           agent.Status,
		Purpose:          agent.Purpose,
		Avatar:           agent.Avatar,
		RegisteredAt:     agent.RegisteredAt,
		LastSeen:         agent.LastSeen,
		LeftAt:           agent.LeftAt,
		Managed:          agent.Managed,
		Invoke:           agent.Invoke,
		Presence:         string(agent.Presence),
		MentionWatermark: agent.MentionWatermark,
	}

	if channelID != "" {
		record.HomeChannel = &channelID
	}

	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgentUpdate appends an agent update record to JSONL.
func AppendAgentUpdate(projectPath string, update AgentUpdateJSONLRecord) error {
	update.Type = "agent_update"
	if err := appendJSONLine(runtimePath(projectPath), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendSessionStart appends a session start event to JSONL.
func AppendSessionStart(projectPath string, event types.SessionStart) error {
	record := SessionStartJSONLRecord{
		Type:        "session_start",
		AgentID:     event.AgentID,
		SessionID:   event.SessionID,
		TriggeredBy: event.TriggeredBy,
		ThreadGUID:  event.ThreadGUID,
		StartedAt:   event.StartedAt,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendSessionEnd appends a session end event to JSONL.
func AppendSessionEnd(projectPath string, event types.SessionEnd) error {
	record := SessionEndJSONLRecord{
		Type:       "session_end",
		AgentID:    event.AgentID,
		SessionID:  event.SessionID,
		ExitCode:   event.ExitCode,
		DurationMs: event.DurationMs,
		EndedAt:    event.EndedAt,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendSessionShutdown appends a graceful shutdown event to JSONL.
func AppendSessionShutdown(projectPath string, event types.SessionShutdown) error {
	record := SessionShutdownJSONLRecord{
		Type:            "session_shutdown",
		AgentID:         event.AgentID,
		SessionID:       event.SessionID,
		UnprocessedMsgs: event.UnprocessedMsgs,
		NewWatermark:    event.NewWatermark,
		ShutdownAt:      event.ShutdownAt,
		ShutdownReason:  event.ShutdownReason,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendUsageSnapshot appends a usage snapshot event to JSONL.
// Called on session end to persist token usage for durability.
func AppendUsageSnapshot(projectPath string, snapshot types.UsageSnapshot) error {
	record := UsageSnapshotJSONLRecord{
		Type:           "usage_snapshot",
		AgentID:        snapshot.AgentID,
		SessionID:      snapshot.SessionID,
		Driver:         snapshot.Driver,
		Model:          snapshot.Model,
		InputTokens:    snapshot.InputTokens,
		OutputTokens:   snapshot.OutputTokens,
		CachedTokens:   snapshot.CachedTokens,
		ContextLimit:   snapshot.ContextLimit,
		ContextPercent: snapshot.ContextPercent,
		CapturedAt:     snapshot.CapturedAt,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendSessionHeartbeat appends a session heartbeat event to JSONL.
func AppendSessionHeartbeat(projectPath string, event types.SessionHeartbeat) error {
	record := SessionHeartbeatJSONLRecord{
		Type:      "session_heartbeat",
		AgentID:   event.AgentID,
		SessionID: event.SessionID,
		Status:    string(event.Status),
		At:        event.At,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendPresenceEvent appends a presence state transition to JSONL for audit trail.
func AppendPresenceEvent(projectPath string, event PresenceEventJSONLRecord) error {
	event.Type = "presence_event"
	if err := appendJSONLine(runtimePath(projectPath), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendGhostCursor appends a ghost cursor event to JSONL.
func AppendGhostCursor(projectPath string, cursor types.GhostCursor) error {
	record := GhostCursorJSONLRecord{
		Type:        "ghost_cursor",
		AgentID:     cursor.AgentID,
		Home:        cursor.Home,
		MessageGUID: cursor.MessageGUID,
		MustRead:    cursor.MustRead,
		SetAt:       cursor.SetAt,
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendCursorClear appends a ghost cursor clear tombstone to JSONL.
func AppendCursorClear(projectPath, agentID, home string, clearedAt int64) error {
	if clearedAt == 0 {
		clearedAt = time.Now().UnixMilli()
	}
	record := CursorClearJSONLRecord{
		Type:    "cursor_clear",
		AgentID: agentID,
		Home:    home,
		TS:      clearedAt,
	}
	if IsMultiMachineMode(projectPath) {
		seq, err := GetNextSequence(projectPath)
		if err != nil {
			return err
		}
		record.Seq = seq
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgentDescriptor appends an agent descriptor event to JSONL.
func AppendAgentDescriptor(projectPath, agentID string, displayName *string, capabilities []string, ts int64) error {
	if ts == 0 {
		ts = time.Now().Unix()
	}
	record := AgentDescriptorJSONLRecord{
		Type:         "agent_descriptor",
		AgentID:      agentID,
		DisplayName:  displayName,
		Capabilities: capabilities,
		TS:           ts,
	}
	if IsMultiMachineMode(projectPath) {
		seq, err := GetNextSequence(projectPath)
		if err != nil {
			return err
		}
		record.Seq = seq
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

func ensureAgentDescriptor(projectPath, agentID string, ts int64) error {
	if !IsMultiMachineMode(projectPath) || agentID == "" {
		return nil
	}
	exists, err := agentDescriptorExists(projectPath, agentID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return AppendAgentDescriptor(projectPath, agentID, nil, nil, ts)
}

func agentDescriptorExists(projectPath, agentID string) (bool, error) {
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return false, err
	}
	lines, err := readJSONLLines(filePath)
	if err != nil {
		return false, err
	}
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type != "agent_descriptor" {
			continue
		}
		var record struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record.AgentID == agentID {
			return true, nil
		}
	}
	return false, nil
}
