package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
)

func TestEnsureLLMRouterCreatesTemplates(t *testing.T) {
	projectDir := t.TempDir()
	if err := ensureLLMRouter(projectDir); err != nil {
		t.Fatalf("ensureLLMRouter: %v", err)
	}

	paths := []string{
		filepath.Join(projectDir, ".fray", "llm", "routers", "mentions.mld"),
		filepath.Join(projectDir, ".fray", "llm", "routers", "stdout-repair.mld"),
		filepath.Join(projectDir, ".fray", "llm", "slash", "fly.mld"),
		filepath.Join(projectDir, ".fray", "llm", "prompts", "mention-fresh.mld"),
		filepath.Join(projectDir, ".fray", "llm", "status.mld"),
		filepath.Join(projectDir, ".fray", "mlld-config.json"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}

func TestCreateManagedAgentAppendsAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	projectDir := t.TempDir()
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
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init command: %v", err)
	}

	project, err := core.DiscoverProject(projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}

	if err := createManagedAgent(project.DBPath, "builder", "claude"); err != nil {
		t.Fatalf("createManagedAgent: %v", err)
	}

	agents, err := db.ReadAgents(project.DBPath)
	if err != nil {
		t.Fatalf("read agents: %v", err)
	}
	found := false
	for _, agent := range agents {
		if agent.AgentID == "builder" {
			found = true
			if !agent.Managed {
				t.Fatalf("expected managed agent")
			}
			if agent.Invoke == nil || agent.Invoke.Driver != "claude" {
				t.Fatalf("expected invoke driver claude, got %+v", agent.Invoke)
			}
		}
	}
	if !found {
		t.Fatalf("expected builder agent in JSONL")
	}
}
