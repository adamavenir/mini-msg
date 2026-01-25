package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

func writeJSONLLines(t *testing.T, path string, records ...any) {
	t.Helper()
	var lines []string
	for _, record := range records {
		switch v := record.(type) {
		case string:
			lines = append(lines, v)
		default:
			data, err := json.Marshal(record)
			if err != nil {
				t.Fatalf("marshal record: %v", err)
			}
			lines = append(lines, string(data))
		}
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return appendNewline(strings.Join(lines, "\n"))
}

func appendNewline(value string) string {
	if value == "" {
		return value
	}
	if value[len(value)-1] == '\n' {
		return value
	}
	return value + "\n"
}

func TestReadReactionsLegacy(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir .fray: %v", err)
	}

	msg := MessageJSONLRecord{
		Type:      "message",
		ID:        "msg-1",
		FromAgent: "alice",
		Body:      "hello",
		MsgType:   types.MessageTypeAgent,
		TS:        10,
	}
	reaction := ReactionJSONLRecord{
		Type:        "reaction",
		MessageGUID: msg.ID,
		AgentID:     "bob",
		Emoji:       ":+1:",
		ReactedAt:   20,
	}
	writeJSONLLines(t, filepath.Join(frayDir, messagesFile), msg, reaction)

	records, err := ReadReactions(projectDir)
	if err != nil {
		t.Fatalf("read reactions: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 reaction, got %d", len(records))
	}
	if records[0].MessageGUID != msg.ID || records[0].Emoji != ":+1:" {
		t.Fatalf("unexpected reaction: %#v", records[0])
	}
}

func TestReadReactionsMergedOrdersByReactedAt(t *testing.T) {
	projectDir := t.TempDir()
	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{StorageVersion: 2}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	machineA := filepath.Join(projectDir, ".fray", "shared", "machines", "alpha")
	machineB := filepath.Join(projectDir, ".fray", "shared", "machines", "beta")
	if err := os.MkdirAll(machineA, 0o755); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}
	if err := os.MkdirAll(machineB, 0o755); err != nil {
		t.Fatalf("mkdir beta: %v", err)
	}

	reactionLater := map[string]any{
		"type":         "reaction",
		"message_guid": "msg-1",
		"agent_id":     "alice",
		"emoji":        ":+1:",
		"reacted_at":   int64(200),
	}
	reactionEarlier := map[string]any{
		"type":         "reaction",
		"message_guid": "msg-1",
		"agent_id":     "bob",
		"emoji":        ":heart:",
		"reacted_at":   int64(100),
	}

	writeJSONLLines(t, filepath.Join(machineA, messagesFile), reactionLater)
	writeJSONLLines(t, filepath.Join(machineB, messagesFile), reactionEarlier)

	records, err := ReadReactions(projectDir)
	if err != nil {
		t.Fatalf("read reactions: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 reactions, got %d", len(records))
	}
	if records[0].Emoji != ":heart:" || records[1].Emoji != ":+1:" {
		t.Fatalf("expected ordering by reacted_at, got %#v", records)
	}
}

func TestReadFavesLegacy(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir .fray: %v", err)
	}

	fave := AgentFaveJSONLRecord{
		Type:     "agent_fave",
		AgentID:  "alice",
		ItemType: "thread",
		ItemGUID: "thrd-1",
		FavedAt:  10,
	}
	unfave := AgentUnfaveJSONLRecord{
		Type:      "agent_unfave",
		AgentID:   "alice",
		ItemType:  "thread",
		ItemGUID:  "thrd-1",
		UnfavedAt: 20,
	}
	writeJSONLLines(t, filepath.Join(frayDir, agentsFile), fave, unfave)

	events, err := ReadFaves(projectDir)
	if err != nil {
		t.Fatalf("read faves: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 fave events, got %d", len(events))
	}
	if events[0].Type != "agent_fave" || events[1].Type != "agent_unfave" {
		t.Fatalf("unexpected fave events: %#v", events)
	}
}

func TestReadRolesLegacy(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir .fray: %v", err)
	}

	hold := RoleHoldJSONLRecord{
		Type:       "role_hold",
		AgentID:    "alice",
		RoleName:   "architect",
		AssignedAt: 100,
	}
	play := RolePlayJSONLRecord{
		Type:      "role_play",
		AgentID:   "alice",
		RoleName:  "driver",
		SessionID: strPtr("sess-1"),
		StartedAt: 120,
	}
	writeJSONLLines(t, filepath.Join(frayDir, agentsFile), hold, play)

	events, err := ReadRoles(projectDir)
	if err != nil {
		t.Fatalf("read roles: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 role events, got %d", len(events))
	}
	if events[0].RoleName != "architect" || events[1].RoleName != "driver" {
		t.Fatalf("unexpected role events: %#v", events)
	}
}

func TestReadPermissionsAppliesUpdates(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir .fray: %v", err)
	}

	req := PermissionJSONLRecord{
		Type:      "permission_request",
		GUID:      "perm-1",
		FromAgent: "alice",
		Tool:      "fs",
		Action:    "read",
		Rationale: "need files",
		Status:    string(types.PermissionStatusPending),
		Options: []types.PermissionOption{{
			Label:    "ok",
			Patterns: []string{"fs:read"},
			Scope:    types.PermissionScopeOnce,
		}},
		CreatedAt: 10,
	}
	update := PermissionUpdateJSONLRecord{
		Type:        "permission_update",
		GUID:        "perm-1",
		Status:      string(types.PermissionStatusApproved),
		ChosenIndex: func() *int { v := 0; return &v }(),
		RespondedBy: "bob",
		RespondedAt: 20,
	}
	writeJSONLLines(t, filepath.Join(frayDir, permissionsFile), req, update)

	perms, err := ReadPermissions(projectDir)
	if err != nil {
		t.Fatalf("read permissions: %v", err)
	}
	if len(perms) != 1 {
		t.Fatalf("expected 1 permission, got %d", len(perms))
	}
	if perms[0].Status != types.PermissionStatusApproved {
		t.Fatalf("expected approved status, got %s", perms[0].Status)
	}
	if perms[0].ChosenIndex == nil || *perms[0].ChosenIndex != 0 {
		t.Fatalf("expected chosen index 0, got %#v", perms[0].ChosenIndex)
	}
	byGUID, err := ReadPermissionByGUID(projectDir, "perm-1")
	if err != nil {
		t.Fatalf("read permission by guid: %v", err)
	}
	if byGUID == nil || byGUID.GUID != "perm-1" {
		t.Fatalf("unexpected permission: %#v", byGUID)
	}
}

func TestReadTriggerEventsOrdersLatestFirst(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir .fray: %v", err)
	}

	start1 := SessionStartJSONLRecord{
		Type:      "session_start",
		AgentID:   "alice",
		SessionID: "sess-1",
		StartedAt: 100,
	}
	start2 := SessionStartJSONLRecord{
		Type:      "session_start",
		AgentID:   "bob",
		SessionID: "sess-2",
		StartedAt: 200,
	}
	end1 := SessionEndJSONLRecord{
		Type:       "session_end",
		AgentID:    "alice",
		SessionID:  "sess-1",
		ExitCode:   0,
		DurationMs: 500,
		EndedAt:    150,
	}

	writeJSONLLines(t, filepath.Join(frayDir, agentsFile), start1, start2, end1)

	events, err := ReadTriggerEvents(projectDir)
	if err != nil {
		t.Fatalf("read trigger events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].SessionID != "sess-2" || events[1].SessionID != "sess-1" {
		t.Fatalf("expected latest session first, got %#v", events)
	}
	if events[1].EndedAt == nil || *events[1].EndedAt != 150 {
		t.Fatalf("expected sess-1 ended_at 150, got %#v", events[1])
	}
}

func TestReadThreadPinsLegacy(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir .fray: %v", err)
	}

	pin := ThreadPinJSONLRecord{
		Type:       "thread_pin",
		ThreadGUID: "thrd-1",
		PinnedBy:   "alice",
		PinnedAt:   10,
	}
	unpin := ThreadUnpinJSONLRecord{
		Type:       "thread_unpin",
		ThreadGUID: "thrd-1",
		UnpinnedAt: 20,
	}
	writeJSONLLines(t, filepath.Join(frayDir, threadsFile), pin, unpin)

	events, err := ReadThreadPins(projectDir)
	if err != nil {
		t.Fatalf("read thread pins: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 pin events, got %d", len(events))
	}
	if events[0].Type != "thread_pin" || events[1].Type != "thread_unpin" {
		t.Fatalf("unexpected pin events: %#v", events)
	}
}

func TestReadThreadMutesLegacy(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir .fray: %v", err)
	}

	mute := ThreadMuteJSONLRecord{
		Type:       "thread_mute",
		ThreadGUID: "thrd-1",
		AgentID:    "alice",
		MutedAt:    10,
	}
	unmute := ThreadUnmuteJSONLRecord{
		Type:       "thread_unmute",
		ThreadGUID: "thrd-1",
		AgentID:    "alice",
		UnmutedAt:  20,
	}
	writeJSONLLines(t, filepath.Join(frayDir, threadsFile), mute, unmute)

	events, err := ReadThreadMutes(projectDir)
	if err != nil {
		t.Fatalf("read thread mutes: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 mute events, got %d", len(events))
	}
	if events[0].Type != "thread_mute" || events[1].Type != "thread_unmute" {
		t.Fatalf("unexpected mute events: %#v", events)
	}
}
