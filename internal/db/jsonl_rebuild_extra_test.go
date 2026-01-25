package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

func TestRebuildDatabaseFromJSONL_ReactionsPinsMutesRolesFaves(t *testing.T) {
	projectDir := t.TempDir()

	agent := types.Agent{
		GUID:         "usr-rebuild-1",
		AgentID:      "alice",
		RegisteredAt: 1,
		LastSeen:     2,
		Managed:      true,
		Invoke: &types.InvokeConfig{
			Driver:         "claude",
			PromptDelivery: types.PromptDeliveryStdin,
		},
	}
	if err := AppendAgent(projectDir, agent); err != nil {
		t.Fatalf("append agent: %v", err)
	}

	thread := types.Thread{
		GUID:      "thrd-rebuild-1",
		Name:      "testing",
		Status:    types.ThreadStatusOpen,
		CreatedAt: 10,
	}
	if err := AppendThread(projectDir, thread, []string{agent.AgentID}); err != nil {
		t.Fatalf("append thread: %v", err)
	}

	message := types.Message{
		ID:        "msg-rebuild-1",
		TS:        20,
		FromAgent: agent.AgentID,
		Body:      "hello",
		Type:      types.MessageTypeAgent,
		Home:      thread.GUID,
	}
	if err := AppendMessage(projectDir, message); err != nil {
		t.Fatalf("append message: %v", err)
	}
	if err := AppendReaction(projectDir, message.ID, "bob", ":+1:", 30); err != nil {
		t.Fatalf("append reaction: %v", err)
	}

	if err := AppendThreadPin(projectDir, ThreadPinJSONLRecord{
		ThreadGUID: thread.GUID,
		PinnedBy:   agent.AgentID,
		PinnedAt:   40,
	}); err != nil {
		t.Fatalf("append thread pin: %v", err)
	}

	if err := AppendThreadMute(projectDir, ThreadMuteJSONLRecord{
		ThreadGUID: thread.GUID,
		AgentID:    agent.AgentID,
		MutedAt:    50,
	}); err != nil {
		t.Fatalf("append thread mute: %v", err)
	}

	if err := AppendAgentFave(projectDir, agent.AgentID, "thread", thread.GUID, 60); err != nil {
		t.Fatalf("append fave: %v", err)
	}
	if err := AppendRoleHold(projectDir, agent.AgentID, "architect", 70); err != nil {
		t.Fatalf("append role hold: %v", err)
	}
	if err := AppendRolePlay(projectDir, agent.AgentID, "driver", strPtr("sess-1"), 80); err != nil {
		t.Fatalf("append role play: %v", err)
	}

	dbConn := openTestDB(t)
	if err := RebuildDatabaseFromJSONL(dbConn, projectDir); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	reactions, err := GetReactionsForMessage(dbConn, message.ID)
	if err != nil {
		t.Fatalf("get reactions: %v", err)
	}
	if len(reactions) == 0 || len(reactions[":+1:"]) != 1 {
		t.Fatalf("expected reaction persisted, got %#v", reactions)
	}

	pinned, err := IsThreadPinned(dbConn, thread.GUID)
	if err != nil {
		t.Fatalf("is thread pinned: %v", err)
	}
	if !pinned {
		t.Fatalf("expected thread pinned after rebuild")
	}

	muted, err := GetMutedThreadGUIDs(dbConn, agent.AgentID)
	if err != nil {
		t.Fatalf("get muted threads: %v", err)
	}
	if !muted[thread.GUID] {
		t.Fatalf("expected thread muted after rebuild")
	}

	faves, err := GetFaves(dbConn, agent.AgentID, "thread")
	if err != nil {
		t.Fatalf("get faves: %v", err)
	}
	if len(faves) != 1 || faves[0].ItemGUID != thread.GUID {
		t.Fatalf("expected fave for thread, got %#v", faves)
	}

	roles, err := GetAgentRoles(dbConn, agent.AgentID)
	if err != nil {
		t.Fatalf("get roles: %v", err)
	}
	if len(roles.Held) != 1 || roles.Held[0] != "architect" {
		t.Fatalf("expected held role architect, got %#v", roles.Held)
	}
	if len(roles.Playing) != 1 || roles.Playing[0] != "driver" {
		t.Fatalf("expected playing role driver, got %#v", roles.Playing)
	}
}

func TestRebuildDatabaseFromJSONL_MultiMachineReactionsAndPins(t *testing.T) {
	projectDir := t.TempDir()
	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{StorageVersion: 2}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	machineDir := filepath.Join(projectDir, ".fray", "shared", "machines", "laptop")
	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		t.Fatalf("mkdir machine: %v", err)
	}

	thread := ThreadJSONLRecord{
		Type:      "thread",
		GUID:      "thrd-multi-1",
		Name:      "multi",
		Status:    string(types.ThreadStatusOpen),
		CreatedAt: 10,
	}
	threadPin := ThreadPinJSONLRecord{
		Type:       "thread_pin",
		ThreadGUID: thread.GUID,
		PinnedBy:   "alice",
		PinnedAt:   20,
	}
	threadData, _ := json.Marshal(thread)
	pinData, _ := json.Marshal(threadPin)
	if err := os.WriteFile(filepath.Join(machineDir, threadsFile), append(append(threadData, '\n'), append(pinData, '\n')...), 0o644); err != nil {
		t.Fatalf("write threads: %v", err)
	}

	message := MessageJSONLRecord{
		Type:      "message",
		ID:        "msg-multi-1",
		FromAgent: "alice",
		Body:      "hello",
		MsgType:   types.MessageTypeAgent,
		TS:        30,
	}
	reaction := ReactionJSONLRecord{
		Type:        "reaction",
		MessageGUID: message.ID,
		AgentID:     "bob",
		Emoji:       ":heart:",
		ReactedAt:   40,
	}
	msgData, _ := json.Marshal(message)
	reactionData, _ := json.Marshal(reaction)
	if err := os.WriteFile(filepath.Join(machineDir, messagesFile), append(append(msgData, '\n'), append(reactionData, '\n')...), 0o644); err != nil {
		t.Fatalf("write messages: %v", err)
	}

	dbConn := openTestDB(t)
	if err := RebuildDatabaseFromJSONL(dbConn, projectDir); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	reactions, err := GetReactionsForMessage(dbConn, message.ID)
	if err != nil {
		t.Fatalf("get reactions: %v", err)
	}
	if len(reactions[":heart:"]) != 1 {
		t.Fatalf("expected heart reaction, got %#v", reactions)
	}

	pinned, err := IsThreadPinned(dbConn, thread.GUID)
	if err != nil {
		t.Fatalf("is thread pinned: %v", err)
	}
	if !pinned {
		t.Fatalf("expected thread pinned after rebuild")
	}
}
