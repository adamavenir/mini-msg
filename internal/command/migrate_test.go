package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func TestMigrateCommandFailsWhenConfigExists(t *testing.T) {
	projectDir := t.TempDir()
	project, err := core.InitProject(projectDir, false)
	if err != nil {
		t.Fatalf("init project: %v", err)
	}

	if err := os.WriteFile(project.DBPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	configPath := filepath.Join(filepath.Dir(project.DBPath), "fray-config.json")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	output, err := executeCommand(cmd, "migrate")
	if err == nil {
		t.Fatalf("expected error, got output: %q", output)
	}
	if !strings.Contains(output, "already migrated") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestMigrateMultiMachineCommand(t *testing.T) {
	projectDir := t.TempDir()
	_, err := core.InitProject(projectDir, false)
	if err != nil {
		t.Fatalf("init project: %v", err)
	}

	frayDir := filepath.Join(projectDir, ".fray")
	message := db.MessageJSONLRecord{
		Type:      "message",
		ID:        "msg-1",
		FromAgent: "alice",
		Body:      "hello",
		Mentions:  []string{},
		MsgType:   types.MessageTypeAgent,
		TS:        100,
	}
	msgData, _ := json.Marshal(message)
	if err := os.WriteFile(filepath.Join(frayDir, "messages.jsonl"), append(msgData, '\n'), 0o644); err != nil {
		t.Fatalf("write messages: %v", err)
	}

	threadData := []byte("{\"type\":\"thread\",\"guid\":\"thrd-1\",\"name\":\"test\",\"status\":\"open\",\"created_at\":10}\n")
	if err := os.WriteFile(filepath.Join(frayDir, "threads.jsonl"), threadData, 0o644); err != nil {
		t.Fatalf("write threads: %v", err)
	}

	questionData := []byte("{\"type\":\"question\",\"guid\":\"qstn-1\",\"re\":\"?\",\"from_agent\":\"alice\",\"status\":\"open\",\"created_at\":1}\n")
	if err := os.WriteFile(filepath.Join(frayDir, "questions.jsonl"), questionData, 0o644); err != nil {
		t.Fatalf("write questions: %v", err)
	}

	agentRecord := db.AgentJSONLRecord{
		Type:         "agent",
		ID:           "usr-1",
		AgentID:      "alice",
		RegisteredAt: 1,
		LastSeen:     2,
	}
	agentData, _ := json.Marshal(agentRecord)
	cursorData := []byte("{\"type\":\"ghost_cursor\",\"agent_id\":\"alice\",\"home\":\"room\",\"message_guid\":\"msg-1\",\"must_read\":true,\"set_at\":5}\n")
	agentsFile := filepath.Join(frayDir, "agents.jsonl")
	if err := os.WriteFile(agentsFile, append(append(agentData, '\n'), cursorData...), 0o644); err != nil {
		t.Fatalf("write agents: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "migrate", "--multi-machine"); err != nil {
		t.Fatalf("migrate --multi-machine: %v", err)
	}

	if _, err := os.Stat(filepath.Join(frayDir, "shared", ".v2")); err != nil {
		t.Fatalf("missing sentinel: %v", err)
	}
	if _, err := os.Stat(filepath.Join(frayDir, "messages.jsonl.v1-migrated")); err != nil {
		t.Fatalf("messages not renamed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(frayDir, "agents.jsonl.v1-migrated")); err != nil {
		t.Fatalf("agents not renamed: %v", err)
	}

	machinesDir := filepath.Join(frayDir, "shared", "machines")
	entries, err := os.ReadDir(machinesDir)
	if err != nil {
		t.Fatalf("read machines: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 machine dir, got %d", len(entries))
	}
	machineDir := filepath.Join(machinesDir, entries[0].Name())
	if _, err := os.Stat(filepath.Join(machineDir, "messages.jsonl")); err != nil {
		t.Fatalf("missing shared messages: %v", err)
	}
	if _, err := os.Stat(filepath.Join(machineDir, "agent-state.jsonl")); err != nil {
		t.Fatalf("missing agent-state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(frayDir, "local", "runtime.jsonl")); err != nil {
		t.Fatalf("missing runtime.jsonl: %v", err)
	}
}
