package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
)

func setupMultiMachineProject(t *testing.T, descriptors []db.AgentDescriptorJSONLRecord) string {
	t.Helper()

	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")

	localDir := filepath.Join(frayDir, "local")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "machine-id"), []byte(`{"id":"local","seq":0,"created_at":1}`), 0o644); err != nil {
		t.Fatalf("write machine-id: %v", err)
	}

	machineDir := filepath.Join(frayDir, "shared", "machines", "remote")
	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		t.Fatalf("mkdir shared machine: %v", err)
	}
	if len(descriptors) > 0 {
		var lines []string
		for _, descriptor := range descriptors {
			data, _ := json.Marshal(descriptor)
			lines = append(lines, string(data))
		}
		if err := os.WriteFile(filepath.Join(machineDir, "agent-state.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("write agent-state: %v", err)
		}
	}

	if err := os.WriteFile(filepath.Join(frayDir, "shared", ".v2"), []byte(""), 0o644); err != nil {
		t.Fatalf("write v2 sentinel: %v", err)
	}

	if _, err := db.UpdateProjectConfig(projectDir, db.ProjectConfig{
		StorageVersion: 2,
		ChannelID:      "ch-agent",
		ChannelName:    "agent-test",
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	project := core.Project{Root: projectDir, DBPath: filepath.Join(frayDir, "fray.db")}
	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.RebuildDatabaseFromJSONL(dbConn, project.DBPath); err != nil {
		_ = dbConn.Close()
		t.Fatalf("rebuild db: %v", err)
	}
	_ = dbConn.Close()

	return projectDir
}

func TestAgentAddRegistersLocally(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := setupMultiMachineProject(t, []db.AgentDescriptorJSONLRecord{
		{Type: "agent_descriptor", AgentID: "alice", TS: 10},
	})

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
	if _, err := executeCommand(cmd, "agent", "add", "alice", "--driver", "claude", "--model", "opus-4"); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	agents, err := db.ReadAgents(projectDir)
	if err != nil {
		t.Fatalf("read agents: %v", err)
	}
	var found *db.AgentJSONLRecord
	for i := range agents {
		if agents[i].AgentID == "alice" {
			found = &agents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected alice in runtime")
	}
	if found.Invoke == nil || found.Invoke.Driver != "claude" || found.Invoke.Model != "opus-4" {
		t.Fatalf("unexpected invoke config: %#v", found.Invoke)
	}
}

func TestAgentRemoveMarksUnmanaged(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := setupMultiMachineProject(t, []db.AgentDescriptorJSONLRecord{
		{Type: "agent_descriptor", AgentID: "alice", TS: 10},
	})

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
	if _, err := executeCommand(cmd, "agent", "add", "alice"); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "agent", "remove", "alice"); err != nil {
		t.Fatalf("agent remove: %v", err)
	}

	agents, err := db.ReadAgents(projectDir)
	if err != nil {
		t.Fatalf("read agents: %v", err)
	}
	var managed *bool
	for i := range agents {
		if agents[i].AgentID == "alice" {
			managed = &agents[i].Managed
		}
	}
	if managed == nil {
		t.Fatalf("expected alice in runtime")
	}
	if *managed {
		t.Fatalf("expected alice to be unmanaged after remove")
	}

	project, err := core.DiscoverProject(projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}
	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer dbConn.Close()

	agent, err := db.GetAgent(dbConn, "alice")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent == nil {
		t.Fatalf("expected alice to remain in db")
	}
}
