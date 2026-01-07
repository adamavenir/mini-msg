package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

func appendJSONLine(filePath string, record any) error {
	if err := ensureDir(filepath.Dir(filePath)); err != nil {
		return err
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}

	return nil
}

// AppendMessage appends a message record to JSONL.
func AppendMessage(projectPath string, message types.Message) error {
	frayDir := resolveFrayDir(projectPath)
	home := message.Home
	if home == "" {
		home = "room"
	}
	// Convert new reactions format to legacy format for JSONL compatibility.
	// Reactions are now stored in separate reaction records, so this is usually empty.
	legacyReactions := ConvertToLegacyReactions(message.Reactions)
	record := MessageJSONLRecord{
		Type:             "message",
		ID:               message.ID,
		ChannelID:        message.ChannelID,
		Home:             home,
		FromAgent:        message.FromAgent,
		Body:             message.Body,
		Mentions:         message.Mentions,
		Reactions:        legacyReactions,
		MsgType:          message.Type,
		References:       message.References,
		SurfaceMessage:   message.SurfaceMessage,
		ReplyTo:          message.ReplyTo,
		QuoteMessageGUID: message.QuoteMessageGUID,
		TS:               message.TS,
		EditedAt:         message.EditedAt,
		ArchivedAt:       message.ArchivedAt,
	}

	if err := appendJSONLine(filepath.Join(frayDir, messagesFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageUpdate appends an update record to JSONL.
func AppendMessageUpdate(projectPath string, update MessageUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "message_update"
	if err := appendJSONLine(filepath.Join(frayDir, messagesFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgent appends an agent record to JSONL.
func AppendAgent(projectPath string, agent types.Agent) error {
	frayDir := resolveFrayDir(projectPath)
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

	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgentUpdate appends an agent update record to JSONL.
func AppendAgentUpdate(projectPath string, update AgentUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "agent_update"
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendSessionStart appends a session start event to JSONL.
func AppendSessionStart(projectPath string, event types.SessionStart) error {
	frayDir := resolveFrayDir(projectPath)
	record := SessionStartJSONLRecord{
		Type:        "session_start",
		AgentID:     event.AgentID,
		SessionID:   event.SessionID,
		TriggeredBy: event.TriggeredBy,
		ThreadGUID:  event.ThreadGUID,
		StartedAt:   event.StartedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendSessionEnd appends a session end event to JSONL.
func AppendSessionEnd(projectPath string, event types.SessionEnd) error {
	frayDir := resolveFrayDir(projectPath)
	record := SessionEndJSONLRecord{
		Type:       "session_end",
		AgentID:    event.AgentID,
		SessionID:  event.SessionID,
		ExitCode:   event.ExitCode,
		DurationMs: event.DurationMs,
		EndedAt:    event.EndedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendSessionHeartbeat appends a session heartbeat event to JSONL.
func AppendSessionHeartbeat(projectPath string, event types.SessionHeartbeat) error {
	frayDir := resolveFrayDir(projectPath)
	record := SessionHeartbeatJSONLRecord{
		Type:      "session_heartbeat",
		AgentID:   event.AgentID,
		SessionID: event.SessionID,
		Status:    string(event.Status),
		At:        event.At,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendQuestion appends a question record to JSONL.
func AppendQuestion(projectPath string, question types.Question) error {
	frayDir := resolveFrayDir(projectPath)
	record := QuestionJSONLRecord{
		Type:       "question",
		GUID:       question.GUID,
		Re:         question.Re,
		FromAgent:  question.FromAgent,
		ToAgent:    question.ToAgent,
		Status:     string(question.Status),
		ThreadGUID: question.ThreadGUID,
		AskedIn:    question.AskedIn,
		AnsweredIn: question.AnsweredIn,
		Options:    question.Options,
		CreatedAt:  question.CreatedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, questionsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendQuestionUpdate appends a question update record to JSONL.
func AppendQuestionUpdate(projectPath string, update QuestionUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "question_update"
	if err := appendJSONLine(filepath.Join(frayDir, questionsFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThread appends a thread record to JSONL.
func AppendThread(projectPath string, thread types.Thread, subscribed []string) error {
	frayDir := resolveFrayDir(projectPath)
	record := ThreadJSONLRecord{
		Type:              "thread",
		GUID:              thread.GUID,
		Name:              thread.Name,
		ParentThread:      thread.ParentThread,
		Subscribed:        subscribed,
		Status:            string(thread.Status),
		ThreadType:        string(thread.Type),
		CreatedAt:         thread.CreatedAt,
		AnchorMessageGUID: thread.AnchorMessageGUID,
		AnchorHidden:      thread.AnchorHidden,
		LastActivityAt:    thread.LastActivityAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUpdate appends a thread update record to JSONL.
func AppendThreadUpdate(projectPath string, update ThreadUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "thread_update"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadSubscribe appends a thread subscribe event to JSONL.
func AppendThreadSubscribe(projectPath string, event ThreadSubscribeJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_subscribe"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnsubscribe appends a thread unsubscribe event to JSONL.
func AppendThreadUnsubscribe(projectPath string, event ThreadUnsubscribeJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_unsubscribe"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMessage appends a thread message membership event to JSONL.
func AppendThreadMessage(projectPath string, event ThreadMessageJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_message"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMessageRemove appends a thread message removal event to JSONL.
func AppendThreadMessageRemove(projectPath string, event ThreadMessageRemoveJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_message_remove"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessagePin appends a message pin event to JSONL.
func AppendMessagePin(projectPath string, event MessagePinJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "message_pin"
	if err := appendJSONLine(filepath.Join(frayDir, messagesFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageUnpin appends a message unpin event to JSONL.
func AppendMessageUnpin(projectPath string, event MessageUnpinJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "message_unpin"
	if err := appendJSONLine(filepath.Join(frayDir, messagesFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageMove appends a message move event to JSONL.
func AppendMessageMove(projectPath string, event MessageMoveJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "message_move"
	if err := appendJSONLine(filepath.Join(frayDir, messagesFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadPin appends a thread pin event to JSONL.
func AppendThreadPin(projectPath string, event ThreadPinJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_pin"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnpin appends a thread unpin event to JSONL.
func AppendThreadUnpin(projectPath string, event ThreadUnpinJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_unpin"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMute appends a thread mute event to JSONL.
func AppendThreadMute(projectPath string, event ThreadMuteJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_mute"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnmute appends a thread unmute event to JSONL.
func AppendThreadUnmute(projectPath string, event ThreadUnmuteJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_unmute"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendGhostCursor appends a ghost cursor event to JSONL.
func AppendGhostCursor(projectPath string, cursor types.GhostCursor) error {
	frayDir := resolveFrayDir(projectPath)
	record := GhostCursorJSONLRecord{
		Type:        "ghost_cursor",
		AgentID:     cursor.AgentID,
		Home:        cursor.Home,
		MessageGUID: cursor.MessageGUID,
		MustRead:    cursor.MustRead,
		SetAt:       cursor.SetAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendReaction appends a reaction record to JSONL.
func AppendReaction(projectPath, messageGUID, agentID, emoji string, reactedAt int64) error {
	frayDir := resolveFrayDir(projectPath)
	record := ReactionJSONLRecord{
		Type:        "reaction",
		MessageGUID: messageGUID,
		AgentID:     agentID,
		Emoji:       emoji,
		ReactedAt:   reactedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, messagesFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgentFave appends a fave record to JSONL.
func AppendAgentFave(projectPath, agentID, itemType, itemGUID string, favedAt int64) error {
	frayDir := resolveFrayDir(projectPath)
	record := AgentFaveJSONLRecord{
		Type:     "agent_fave",
		AgentID:  agentID,
		ItemType: itemType,
		ItemGUID: itemGUID,
		FavedAt:  favedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgentUnfave appends an unfave record to JSONL.
func AppendAgentUnfave(projectPath, agentID, itemType, itemGUID string, unfavedAt int64) error {
	frayDir := resolveFrayDir(projectPath)
	record := AgentUnfaveJSONLRecord{
		Type:      "agent_unfave",
		AgentID:   agentID,
		ItemType:  itemType,
		ItemGUID:  itemGUID,
		UnfavedAt: unfavedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRoleHold appends a role hold (persistent assignment) record to JSONL.
func AppendRoleHold(projectPath, agentID, roleName string, assignedAt int64) error {
	frayDir := resolveFrayDir(projectPath)
	record := RoleHoldJSONLRecord{
		Type:       "role_hold",
		AgentID:    agentID,
		RoleName:   roleName,
		AssignedAt: assignedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRoleDrop appends a role drop (removal) record to JSONL.
func AppendRoleDrop(projectPath, agentID, roleName string, droppedAt int64) error {
	frayDir := resolveFrayDir(projectPath)
	record := RoleDropJSONLRecord{
		Type:      "role_drop",
		AgentID:   agentID,
		RoleName:  roleName,
		DroppedAt: droppedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRolePlay appends a session-scoped role play record to JSONL.
func AppendRolePlay(projectPath, agentID, roleName string, sessionID *string, startedAt int64) error {
	frayDir := resolveFrayDir(projectPath)
	record := RolePlayJSONLRecord{
		Type:      "role_play",
		AgentID:   agentID,
		RoleName:  roleName,
		SessionID: sessionID,
		StartedAt: startedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRoleStop appends a role stop record to JSONL.
func AppendRoleStop(projectPath, agentID, roleName string, stoppedAt int64) error {
	frayDir := resolveFrayDir(projectPath)
	record := RoleStopJSONLRecord{
		Type:      "role_stop",
		AgentID:   agentID,
		RoleName:  roleName,
		StoppedAt: stoppedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
