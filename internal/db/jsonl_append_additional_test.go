package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

func lastJSONLType(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected jsonl lines in %s", path)
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return envelope.Type
}

func TestAppendMessageFamilyWritesTypes(t *testing.T) {
	projectDir := t.TempDir()

	message := types.Message{
		ID:        "msg-append-1",
		TS:        10,
		FromAgent: "alice",
		Body:      "hello",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	}

	if err := AppendMessage(projectDir, message); err != nil {
		t.Fatalf("append message: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", messagesFile)); got != "message" {
		t.Fatalf("expected message type, got %s", got)
	}

	body := "updated"
	if err := AppendMessageUpdate(projectDir, MessageUpdateJSONLRecord{ID: message.ID, Body: &body}); err != nil {
		t.Fatalf("append message update: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", messagesFile)); got != "message_update" {
		t.Fatalf("expected message_update type, got %s", got)
	}

	if err := AppendMessagePin(projectDir, MessagePinJSONLRecord{
		MessageGUID: message.ID,
		ThreadGUID:  "thrd-1",
		PinnedBy:    "alice",
		PinnedAt:    20,
	}); err != nil {
		t.Fatalf("append message pin: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", messagesFile)); got != "message_pin" {
		t.Fatalf("expected message_pin type, got %s", got)
	}

	if err := AppendMessageUnpin(projectDir, MessageUnpinJSONLRecord{
		MessageGUID: message.ID,
		ThreadGUID:  "thrd-1",
		UnpinnedBy:  "alice",
		UnpinnedAt:  30,
	}); err != nil {
		t.Fatalf("append message unpin: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", messagesFile)); got != "message_unpin" {
		t.Fatalf("expected message_unpin type, got %s", got)
	}

	if err := AppendMessageMove(projectDir, MessageMoveJSONLRecord{
		MessageGUID: message.ID,
		OldHome:     "room",
		NewHome:     "thrd-1",
		MovedBy:     "alice",
		MovedAt:     40,
	}); err != nil {
		t.Fatalf("append message move: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", messagesFile)); got != "message_move" {
		t.Fatalf("expected message_move type, got %s", got)
	}

	if err := AppendMessageDelete(projectDir, message.ID, strPtr("alice"), 50); err != nil {
		t.Fatalf("append message delete: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", messagesFile)); got != "message_delete" {
		t.Fatalf("expected message_delete type, got %s", got)
	}
}

func TestAppendThreadFamilyWritesTypes(t *testing.T) {
	projectDir := t.TempDir()

	thread := types.Thread{
		GUID:      "thrd-append-1",
		Name:      "thread",
		Status:    types.ThreadStatusOpen,
		CreatedAt: 10,
	}

	if err := AppendThread(projectDir, thread, []string{"alice"}); err != nil {
		t.Fatalf("append thread: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread" {
		t.Fatalf("expected thread type, got %s", got)
	}

	status := string(types.ThreadStatusArchived)
	if err := AppendThreadUpdate(projectDir, ThreadUpdateJSONLRecord{GUID: thread.GUID, Status: &status}); err != nil {
		t.Fatalf("append thread update: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread_update" {
		t.Fatalf("expected thread_update type, got %s", got)
	}

	if err := AppendThreadDelete(projectDir, thread.GUID, 20); err != nil {
		t.Fatalf("append thread delete: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread_delete" {
		t.Fatalf("expected thread_delete type, got %s", got)
	}

	if err := AppendThreadSubscribe(projectDir, ThreadSubscribeJSONLRecord{
		ThreadGUID:   thread.GUID,
		AgentID:      "alice",
		SubscribedAt: 30,
	}); err != nil {
		t.Fatalf("append thread subscribe: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread_subscribe" {
		t.Fatalf("expected thread_subscribe type, got %s", got)
	}

	if err := AppendThreadMessage(projectDir, ThreadMessageJSONLRecord{
		ThreadGUID:  thread.GUID,
		MessageGUID: "msg-1",
		AddedBy:     "alice",
		AddedAt:     40,
	}); err != nil {
		t.Fatalf("append thread message: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread_message" {
		t.Fatalf("expected thread_message type, got %s", got)
	}

	if err := AppendThreadPin(projectDir, ThreadPinJSONLRecord{
		ThreadGUID: thread.GUID,
		PinnedBy:   "alice",
		PinnedAt:   50,
	}); err != nil {
		t.Fatalf("append thread pin: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread_pin" {
		t.Fatalf("expected thread_pin type, got %s", got)
	}

	if err := AppendThreadUnpin(projectDir, ThreadUnpinJSONLRecord{
		ThreadGUID: thread.GUID,
		UnpinnedBy: "alice",
		UnpinnedAt: 60,
	}); err != nil {
		t.Fatalf("append thread unpin: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread_unpin" {
		t.Fatalf("expected thread_unpin type, got %s", got)
	}

	if err := AppendThreadMute(projectDir, ThreadMuteJSONLRecord{
		ThreadGUID: thread.GUID,
		AgentID:    "alice",
		MutedAt:    70,
	}); err != nil {
		t.Fatalf("append thread mute: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread_mute" {
		t.Fatalf("expected thread_mute type, got %s", got)
	}

	if err := AppendThreadUnmute(projectDir, ThreadUnmuteJSONLRecord{
		ThreadGUID: thread.GUID,
		AgentID:    "alice",
		UnmutedAt:  80,
	}); err != nil {
		t.Fatalf("append thread unmute: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", threadsFile)); got != "thread_unmute" {
		t.Fatalf("expected thread_unmute type, got %s", got)
	}
}

func TestAppendAgentAndSessionWritesTypes(t *testing.T) {
	projectDir := t.TempDir()

	agent := types.Agent{
		GUID:         "usr-append-1",
		AgentID:      "alice",
		RegisteredAt: 10,
		LastSeen:     20,
	}
	if err := AppendAgent(projectDir, agent); err != nil {
		t.Fatalf("append agent: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "agent" {
		t.Fatalf("expected agent type, got %s", got)
	}

	if err := AppendAgentUpdate(projectDir, AgentUpdateJSONLRecord{AgentID: agent.AgentID, Status: strPtr("active")}); err != nil {
		t.Fatalf("append agent update: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "agent_update" {
		t.Fatalf("expected agent_update type, got %s", got)
	}

	if err := AppendSessionStart(projectDir, types.SessionStart{
		AgentID:   agent.AgentID,
		SessionID: "sess-1",
		StartedAt: 30,
	}); err != nil {
		t.Fatalf("append session start: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "session_start" {
		t.Fatalf("expected session_start type, got %s", got)
	}

	if err := AppendSessionEnd(projectDir, types.SessionEnd{
		AgentID:    agent.AgentID,
		SessionID:  "sess-1",
		ExitCode:   0,
		DurationMs: 100,
		EndedAt:    40,
	}); err != nil {
		t.Fatalf("append session end: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "session_end" {
		t.Fatalf("expected session_end type, got %s", got)
	}

	if err := AppendSessionHeartbeat(projectDir, types.SessionHeartbeat{
		AgentID:   agent.AgentID,
		SessionID: "sess-1",
		Status:    "active",
		At:        50,
	}); err != nil {
		t.Fatalf("append session heartbeat: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "session_heartbeat" {
		t.Fatalf("expected session_heartbeat type, got %s", got)
	}

	if err := AppendPresenceEvent(projectDir, PresenceEventJSONLRecord{
		AgentID: agent.AgentID,
		From:    "active",
		To:      "idle",
		Reason:  "test",
		Source:  "unit",
		TS:      60,
	}); err != nil {
		t.Fatalf("append presence event: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "presence_event" {
		t.Fatalf("expected presence_event type, got %s", got)
	}

	if err := AppendUsageSnapshot(projectDir, types.UsageSnapshot{
		AgentID:        agent.AgentID,
		SessionID:      "sess-1",
		Driver:         "claude",
		InputTokens:    10,
		OutputTokens:   5,
		CachedTokens:   2,
		ContextLimit:   200000,
		ContextPercent: 1,
		CapturedAt:     time.Now().Unix(),
	}); err != nil {
		t.Fatalf("append usage snapshot: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "usage_snapshot" {
		t.Fatalf("expected usage_snapshot type, got %s", got)
	}
}

func TestAppendQuestionWritesTypes(t *testing.T) {
	projectDir := t.TempDir()

	question := types.Question{
		GUID:      "qstn-1",
		Re:        "test?",
		FromAgent: "alice",
		Status:    types.QuestionStatusOpen,
		CreatedAt: 10,
	}
	if err := AppendQuestion(projectDir, question); err != nil {
		t.Fatalf("append question: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", questionsFile)); got != "question" {
		t.Fatalf("expected question type, got %s", got)
	}

	status := string(types.QuestionStatusAnswered)
	if err := AppendQuestionUpdate(projectDir, QuestionUpdateJSONLRecord{
		GUID:   question.GUID,
		Status: &status,
	}); err != nil {
		t.Fatalf("append question update: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", questionsFile)); got != "question_update" {
		t.Fatalf("expected question_update type, got %s", got)
	}
}

func TestAppendOtherFamiliesWriteTypes(t *testing.T) {
	projectDir := t.TempDir()

	if err := AppendReaction(projectDir, "msg-1", "alice", ":+1:", 10); err != nil {
		t.Fatalf("append reaction: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", messagesFile)); got != "reaction" {
		t.Fatalf("expected reaction type, got %s", got)
	}

	if err := AppendAgentFave(projectDir, "alice", "thread", "thrd-1", 20); err != nil {
		t.Fatalf("append fave: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "agent_fave" {
		t.Fatalf("expected agent_fave type, got %s", got)
	}

	if err := AppendRoleHold(projectDir, "alice", "architect", 30); err != nil {
		t.Fatalf("append role hold: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "role_hold" {
		t.Fatalf("expected role_hold type, got %s", got)
	}

	if err := AppendPermissionRequest(projectDir, types.PermissionRequest{
		GUID:      "perm-1",
		FromAgent: "alice",
		Tool:      "fs",
		Action:    "read",
		Rationale: "test",
		Status:    types.PermissionStatusPending,
		Options: []types.PermissionOption{{
			Label:    "ok",
			Patterns: []string{"fs:read"},
			Scope:    types.PermissionScopeOnce,
		}},
		CreatedAt: 10,
	}); err != nil {
		t.Fatalf("append permission request: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", permissionsFile)); got != "permission_request" {
		t.Fatalf("expected permission_request type, got %s", got)
	}

	if err := AppendWakeCondition(projectDir, types.WakeCondition{
		GUID:        "wake-1",
		AgentID:     "alice",
		Type:        types.WakeConditionAfter,
		AfterMs:     intPtr(1000),
		PersistMode: types.WakePersist,
		CreatedAt:   10,
	}); err != nil {
		t.Fatalf("append wake condition: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "wake_condition" {
		t.Fatalf("expected wake_condition type, got %s", got)
	}

	if err := AppendJobCreate(projectDir, types.Job{
		GUID:       "job-1",
		Name:       "test",
		Status:     types.JobStatusRunning,
		OwnerAgent: "alice",
		ThreadGUID: "thrd-1",
		CreatedAt:  10,
	}); err != nil {
		t.Fatalf("append job create: %v", err)
	}
	if got := lastJSONLType(t, filepath.Join(projectDir, ".fray", agentsFile)); got != "job_create" {
		t.Fatalf("expected job_create type, got %s", got)
	}
}
