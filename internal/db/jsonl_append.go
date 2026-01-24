package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/adamavenir/fray/internal/core"
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
	return atomicAppend(filePath, data)
}

func appendSharedJSONLine(projectPath, filePath string, record any) error {
	if err := appendJSONLine(filePath, record); err != nil {
		return err
	}
	if IsMultiMachineMode(projectPath) {
		if err := updateChecksum(projectPath, filePath); err != nil {
			return err
		}
	}
	return nil
}

func atomicAppend(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}

	return f.Sync()
}

func sharedMachinePath(projectPath, fileName string) (string, error) {
	if !IsMultiMachineMode(projectPath) {
		frayDir := resolveFrayDir(projectPath)
		return filepath.Join(frayDir, fileName), nil
	}
	dir := GetLocalMachineDir(projectPath)
	if dir == "" {
		return "", fmt.Errorf("local machine id not set")
	}
	return filepath.Join(dir, fileName), nil
}

func agentStatePath(projectPath string) (string, error) {
	if !IsMultiMachineMode(projectPath) {
		frayDir := resolveFrayDir(projectPath)
		return filepath.Join(frayDir, agentsFile), nil
	}
	return sharedMachinePath(projectPath, agentStateFile)
}

func runtimePath(projectPath string) string {
	if IsMultiMachineMode(projectPath) {
		return GetLocalRuntimePath(projectPath)
	}
	frayDir := resolveFrayDir(projectPath)
	return filepath.Join(frayDir, agentsFile)
}

// AppendMessage appends a message record to JSONL.
func AppendMessage(projectPath string, message types.Message) error {
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
		SessionID:        message.SessionID,
		Body:             message.Body,
		Mentions:         message.Mentions,
		ForkSessions:     message.ForkSessions,
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

	if IsMultiMachineMode(projectPath) {
		config, err := ReadProjectConfig(projectPath)
		if err != nil {
			return err
		}
		var aliases map[string]string
		if config != nil {
			aliases = config.MachineAliases
		}
		origin := GetLocalMachineID(projectPath)
		if origin == "" {
			return fmt.Errorf("local machine id not set")
		}
		seq, err := GetNextSequence(projectPath)
		if err != nil {
			return err
		}
		record.Origin = origin
		record.Seq = seq
		record.Mentions = core.EncodeMentions(record.Mentions, origin, aliases)
		record.ForkSessions = core.EncodeForkSessions(record.ForkSessions, origin, aliases)
	}

	filePath, err := sharedMachinePath(projectPath, messagesFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageUpdate appends an update record to JSONL.
func AppendMessageUpdate(projectPath string, update MessageUpdateJSONLRecord) error {
	update.Type = "message_update"
	filePath, err := sharedMachinePath(projectPath, messagesFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

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

// AppendQuestion appends a question record to JSONL.
func AppendQuestion(projectPath string, question types.Question) error {
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
	filePath, err := sharedMachinePath(projectPath, questionsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendQuestionUpdate appends a question update record to JSONL.
func AppendQuestionUpdate(projectPath string, update QuestionUpdateJSONLRecord) error {
	update.Type = "question_update"
	filePath, err := sharedMachinePath(projectPath, questionsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThread appends a thread record to JSONL.
func AppendThread(projectPath string, thread types.Thread, subscribed []string) error {
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
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUpdate appends a thread update record to JSONL.
func AppendThreadUpdate(projectPath string, update ThreadUpdateJSONLRecord) error {
	update.Type = "thread_update"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadSubscribe appends a thread subscribe event to JSONL.
func AppendThreadSubscribe(projectPath string, event ThreadSubscribeJSONLRecord) error {
	event.Type = "thread_subscribe"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnsubscribe appends a thread unsubscribe event to JSONL.
func AppendThreadUnsubscribe(projectPath string, event ThreadUnsubscribeJSONLRecord) error {
	event.Type = "thread_unsubscribe"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMessage appends a thread message membership event to JSONL.
func AppendThreadMessage(projectPath string, event ThreadMessageJSONLRecord) error {
	event.Type = "thread_message"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMessageRemove appends a thread message removal event to JSONL.
func AppendThreadMessageRemove(projectPath string, event ThreadMessageRemoveJSONLRecord) error {
	event.Type = "thread_message_remove"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessagePin appends a message pin event to JSONL.
func AppendMessagePin(projectPath string, event MessagePinJSONLRecord) error {
	event.Type = "message_pin"
	var filePath string
	var err error
	if IsMultiMachineMode(projectPath) {
		filePath, err = sharedMachinePath(projectPath, threadsFile)
	} else {
		frayDir := resolveFrayDir(projectPath)
		filePath = filepath.Join(frayDir, messagesFile)
	}
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageUnpin appends a message unpin event to JSONL.
func AppendMessageUnpin(projectPath string, event MessageUnpinJSONLRecord) error {
	event.Type = "message_unpin"
	var filePath string
	var err error
	if IsMultiMachineMode(projectPath) {
		filePath, err = sharedMachinePath(projectPath, threadsFile)
	} else {
		frayDir := resolveFrayDir(projectPath)
		filePath = filepath.Join(frayDir, messagesFile)
	}
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageMove appends a message move event to JSONL.
func AppendMessageMove(projectPath string, event MessageMoveJSONLRecord) error {
	event.Type = "message_move"
	filePath, err := sharedMachinePath(projectPath, messagesFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadPin appends a thread pin event to JSONL.
func AppendThreadPin(projectPath string, event ThreadPinJSONLRecord) error {
	event.Type = "thread_pin"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnpin appends a thread unpin event to JSONL.
func AppendThreadUnpin(projectPath string, event ThreadUnpinJSONLRecord) error {
	event.Type = "thread_unpin"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMute appends a thread mute event to JSONL.
func AppendThreadMute(projectPath string, event ThreadMuteJSONLRecord) error {
	event.Type = "thread_mute"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnmute appends a thread unmute event to JSONL.
func AppendThreadUnmute(projectPath string, event ThreadUnmuteJSONLRecord) error {
	event.Type = "thread_unmute"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
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

// AppendReaction appends a reaction record to JSONL.
func AppendReaction(projectPath, messageGUID, agentID, emoji string, reactedAt int64) error {
	record := ReactionJSONLRecord{
		Type:        "reaction",
		MessageGUID: messageGUID,
		AgentID:     agentID,
		Emoji:       emoji,
		ReactedAt:   reactedAt,
	}
	filePath, err := sharedMachinePath(projectPath, messagesFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgentFave appends a fave record to JSONL.
func AppendAgentFave(projectPath, agentID, itemType, itemGUID string, favedAt int64) error {
	record := AgentFaveJSONLRecord{
		Type:     "agent_fave",
		AgentID:  agentID,
		ItemType: itemType,
		ItemGUID: itemGUID,
		FavedAt:  favedAt,
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

// AppendAgentUnfave appends an unfave record to JSONL.
func AppendAgentUnfave(projectPath, agentID, itemType, itemGUID string, unfavedAt int64) error {
	record := AgentUnfaveJSONLRecord{
		Type:      "agent_unfave",
		AgentID:   agentID,
		ItemType:  itemType,
		ItemGUID:  itemGUID,
		UnfavedAt: unfavedAt,
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

// AppendRoleHold appends a role hold (persistent assignment) record to JSONL.
func AppendRoleHold(projectPath, agentID, roleName string, assignedAt int64) error {
	record := RoleHoldJSONLRecord{
		Type:       "role_hold",
		AgentID:    agentID,
		RoleName:   roleName,
		AssignedAt: assignedAt,
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

// AppendRoleDrop appends a role drop (removal) record to JSONL.
func AppendRoleDrop(projectPath, agentID, roleName string, droppedAt int64) error {
	record := RoleDropJSONLRecord{
		Type:      "role_drop",
		AgentID:   agentID,
		RoleName:  roleName,
		DroppedAt: droppedAt,
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

// AppendRolePlay appends a session-scoped role play record to JSONL.
func AppendRolePlay(projectPath, agentID, roleName string, sessionID *string, startedAt int64) error {
	record := RolePlayJSONLRecord{
		Type:      "role_play",
		AgentID:   agentID,
		RoleName:  roleName,
		SessionID: sessionID,
		StartedAt: startedAt,
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

// AppendRoleStop appends a role stop record to JSONL.
func AppendRoleStop(projectPath, agentID, roleName string, stoppedAt int64) error {
	record := RoleStopJSONLRecord{
		Type:      "role_stop",
		AgentID:   agentID,
		RoleName:  roleName,
		StoppedAt: stoppedAt,
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

// AppendWakeCondition appends a wake condition record to JSONL.
func AppendWakeCondition(projectPath string, condition types.WakeCondition) error {
	record := WakeConditionJSONLRecord{
		Type:           "wake_condition",
		GUID:           condition.GUID,
		AgentID:        condition.AgentID,
		SetBy:          condition.SetBy,
		CondType:       string(condition.Type),
		Pattern:        condition.Pattern,
		OnAgents:       condition.OnAgents,
		InThread:       condition.InThread,
		AfterMs:        condition.AfterMs,
		UseRouter:      condition.UseRouter,
		Prompt:         condition.Prompt,
		PromptText:     condition.PromptText,
		PollIntervalMs: condition.PollIntervalMs,
		PersistMode:    string(condition.PersistMode),
		Paused:         condition.Paused,
		CreatedAt:      condition.CreatedAt,
		ExpiresAt:      condition.ExpiresAt,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionClear appends a wake condition clear record to JSONL.
func AppendWakeConditionClear(projectPath, agentID string) error {
	record := WakeConditionClearJSONLRecord{
		Type:      "wake_condition_clear",
		AgentID:   agentID,
		ClearedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionDelete appends a wake condition delete record to JSONL.
func AppendWakeConditionDelete(projectPath, guid string) error {
	record := WakeConditionDeleteJSONLRecord{
		Type:      "wake_condition_delete",
		GUID:      guid,
		DeletedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionPause appends a wake condition pause record to JSONL.
func AppendWakeConditionPause(projectPath, agentID string) error {
	record := WakeConditionPauseJSONLRecord{
		Type:     "wake_condition_pause",
		AgentID:  agentID,
		PausedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionResume appends a wake condition resume record to JSONL.
func AppendWakeConditionResume(projectPath, agentID string) error {
	record := WakeConditionResumeJSONLRecord{
		Type:      "wake_condition_resume",
		AgentID:   agentID,
		ResumedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionClearByBye appends a wake condition clear-by-bye record to JSONL.
func AppendWakeConditionClearByBye(projectPath, agentID string) error {
	record := WakeConditionClearByByeJSONLRecord{
		Type:      "wake_condition_clear_by_bye",
		AgentID:   agentID,
		ClearedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionReset appends a wake condition reset record to JSONL.
func AppendWakeConditionReset(projectPath, guid string, expiresAt int64) error {
	record := WakeConditionResetJSONLRecord{
		Type:      "wake_condition_reset",
		GUID:      guid,
		ExpiresAt: expiresAt,
		ResetAt:   time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendJobCreate appends a job creation record to JSONL.
func AppendJobCreate(projectPath string, job types.Job) error {
	record := JobCreateJSONLRecord{
		Type:       "job_create",
		GUID:       job.GUID,
		Name:       job.Name,
		Context:    job.Context,
		OwnerAgent: job.OwnerAgent,
		Status:     string(job.Status),
		ThreadGUID: job.ThreadGUID,
		CreatedAt:  job.CreatedAt,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendJobUpdate appends a job update record to JSONL.
func AppendJobUpdate(projectPath string, record JobUpdateJSONLRecord) error {
	record.Type = "job_update"
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendJobWorkerJoin appends a worker join record to JSONL.
func AppendJobWorkerJoin(projectPath string, record JobWorkerJoinJSONLRecord) error {
	record.Type = "job_worker_join"
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendJobWorkerLeave appends a worker leave record to JSONL.
func AppendJobWorkerLeave(projectPath string, record JobWorkerLeaveJSONLRecord) error {
	record.Type = "job_worker_leave"
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendPermissionRequest appends a permission request record to JSONL.
func AppendPermissionRequest(projectPath string, req types.PermissionRequest) error {
	frayDir := resolveFrayDir(projectPath)
	record := PermissionJSONLRecord{
		Type:        "permission_request",
		GUID:        req.GUID,
		FromAgent:   req.FromAgent,
		SessionID:   req.SessionID,
		Tool:        req.Tool,
		Action:      req.Action,
		Rationale:   req.Rationale,
		Options:     req.Options,
		Status:      string(req.Status),
		ChosenIndex: req.ChosenIndex,
		RespondedBy: req.RespondedBy,
		CreatedAt:   req.CreatedAt,
		RespondedAt: req.RespondedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, permissionsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendPermissionUpdate appends a permission response record to JSONL.
func AppendPermissionUpdate(projectPath string, update PermissionUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "permission_update"
	if err := appendJSONLine(filepath.Join(frayDir, permissionsFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
