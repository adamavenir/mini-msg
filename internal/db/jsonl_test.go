package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/adamavenir/mini-msg/internal/types"
)

func TestAppendAndReadMessages(t *testing.T) {
	projectDir := t.TempDir()

	message := types.Message{
		ID:        "msg-abc12345",
		TS:        123,
		ChannelID: strPtr("ch-00000000"),
		FromAgent: "alice.1",
		Body:      "hello",
		Mentions:  []string{"bob.1"},
		Type:      types.MessageTypeAgent,
	}

	if err := AppendMessage(projectDir, message); err != nil {
		t.Fatalf("append message: %v", err)
	}

	readBack, err := ReadMessages(projectDir)
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	if len(readBack) != 1 {
		t.Fatalf("expected 1 message, got %d", len(readBack))
	}
	if readBack[0].ID != message.ID {
		t.Fatalf("expected id %s, got %s", message.ID, readBack[0].ID)
	}
	if len(readBack[0].Mentions) != 1 || readBack[0].Mentions[0] != "bob.1" {
		t.Fatalf("expected mentions to roundtrip")
	}
	if readBack[0].ChannelID == nil || *readBack[0].ChannelID != "ch-00000000" {
		t.Fatalf("expected channel id to roundtrip")
	}
}

func TestAppendAndReadAgents(t *testing.T) {
	projectDir := t.TempDir()

	agent := types.Agent{
		GUID:         "usr-abc12345",
		AgentID:      "alice.1",
		Status:       strPtr("test"),
		Purpose:      nil,
		RegisteredAt: 100,
		LastSeen:     120,
		LeftAt:       nil,
	}

	if err := AppendAgent(projectDir, agent); err != nil {
		t.Fatalf("append agent: %v", err)
	}

	readBack, err := ReadAgents(projectDir)
	if err != nil {
		t.Fatalf("read agents: %v", err)
	}
	if len(readBack) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(readBack))
	}
	if readBack[0].ID != agent.GUID {
		t.Fatalf("expected id %s, got %s", agent.GUID, readBack[0].ID)
	}
	if readBack[0].AgentID != agent.AgentID {
		t.Fatalf("expected agent id %s, got %s", agent.AgentID, readBack[0].AgentID)
	}
}

func TestUpdateProjectConfigMergesKnownAgents(t *testing.T) {
	projectDir := t.TempDir()

	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{
		ChannelID:   "ch-11111111",
		ChannelName: "Alpha",
		KnownAgents: map[string]ProjectKnownAgent{
			"usr-aaa11111": {Name: strPtr("alice.1")},
		},
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{
		KnownAgents: map[string]ProjectKnownAgent{
			"usr-aaa11111": {GlobalName: strPtr("alpha-alice")},
			"usr-bbb22222": {Name: strPtr("bob.1")},
		},
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	config, err := ReadProjectConfig(projectDir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if config == nil {
		t.Fatal("expected config")
	}
	if config.ChannelID != "ch-11111111" {
		t.Fatalf("expected channel id set")
	}
	if config.KnownAgents["usr-aaa11111"].Name == nil || *config.KnownAgents["usr-aaa11111"].Name != "alice.1" {
		t.Fatalf("expected known agent name to persist")
	}
	if config.KnownAgents["usr-aaa11111"].GlobalName == nil || *config.KnownAgents["usr-aaa11111"].GlobalName != "alpha-alice" {
		t.Fatalf("expected global name to merge")
	}
	if config.KnownAgents["usr-bbb22222"].Name == nil || *config.KnownAgents["usr-bbb22222"].Name != "bob.1" {
		t.Fatalf("expected second agent")
	}
}

func TestReadMessagesSkipsMalformedLines(t *testing.T) {
	projectDir := t.TempDir()
	mmDir := filepath.Join(projectDir, ".mm")
	if err := os.MkdirAll(mmDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(mmDir, messagesFile)

	good1, _ := json.Marshal(map[string]any{"type": "message", "id": "msg-good1", "mentions": []string{}})
	good2, _ := json.Marshal(map[string]any{"type": "message", "id": "msg-good2", "mentions": []string{}})
	contents := string(good1) + "\n" + "not-json\n" + string(good2) + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	readBack, err := ReadMessages(projectDir)
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	if len(readBack) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(readBack))
	}
	if readBack[0].ID != "msg-good1" || readBack[1].ID != "msg-good2" {
		t.Fatalf("unexpected ids: %s, %s", readBack[0].ID, readBack[1].ID)
	}
}
