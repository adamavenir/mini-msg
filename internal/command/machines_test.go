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

func TestMachinesCommandListsMachines(t *testing.T) {
	projectDir := setupMachinesProject(t)

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
	output, err := executeCommand(cmd, "machines")
	if err != nil {
		t.Fatalf("machines: %v", err)
	}
	if !strings.Contains(output, "laptop") || !strings.Contains(output, "local") {
		t.Fatalf("expected laptop local in output: %s", output)
	}
	if !strings.Contains(output, "server") || !strings.Contains(output, "remote") {
		t.Fatalf("expected server remote in output: %s", output)
	}
	if !strings.Contains(output, "opus") || !strings.Contains(output, "designer") {
		t.Fatalf("expected agents in output: %s", output)
	}
}

func TestMachinesCommandJSON(t *testing.T) {
	projectDir := setupMachinesProject(t)

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
	output, err := executeCommand(cmd, "--json", "machines")
	if err != nil {
		t.Fatalf("machines --json: %v", err)
	}
	var payload struct {
		Machines []machineInfo `json:"machines"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if len(payload.Machines) != 2 {
		t.Fatalf("expected 2 machines, got %d", len(payload.Machines))
	}
}

func setupMachinesProject(t *testing.T) string {
	t.Helper()

	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	localDir := filepath.Join(frayDir, "local")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "machine-id"), []byte(`{"id":"laptop","seq":0,"created_at":1}`), 0o644); err != nil {
		t.Fatalf("write machine-id: %v", err)
	}

	sharedDir := filepath.Join(frayDir, "shared")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("mkdir shared: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, ".v2"), []byte(""), 0o644); err != nil {
		t.Fatalf("write v2 sentinel: %v", err)
	}

	laptopDir := filepath.Join(frayDir, "shared", "machines", "laptop")
	serverDir := filepath.Join(frayDir, "shared", "machines", "server")
	if err := os.MkdirAll(laptopDir, 0o755); err != nil {
		t.Fatalf("mkdir laptop: %v", err)
	}
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatalf("mkdir server: %v", err)
	}

	laptopMsg := `{"type":"message","id":"msg-l1","from_agent":"opus","body":"hi","ts":10}`
	serverMsg := `{"type":"message","id":"msg-s1","from_agent":"designer","body":"hey","ts":20}`
	if err := os.WriteFile(filepath.Join(laptopDir, "messages.jsonl"), []byte(laptopMsg+"\n"), 0o644); err != nil {
		t.Fatalf("write laptop messages: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "messages.jsonl"), []byte(serverMsg+"\n"), 0o644); err != nil {
		t.Fatalf("write server messages: %v", err)
	}

	if _, err := db.UpdateProjectConfig(projectDir, db.ProjectConfig{
		StorageVersion: 2,
		ChannelID:      "ch-machines",
		ChannelName:    "machines-test",
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
