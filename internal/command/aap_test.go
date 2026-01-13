package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adamavenir/fray/internal/aap"
	"github.com/adamavenir/fray/internal/db"
)

func TestNewCreatesAAPIdentity(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

	// Initialize project
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create agent
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "testuser", "hello world"); err != nil {
		t.Fatalf("new: %v", err)
	}

	// Verify fray agent was created
	dbConn := openProjectDB(t, projectDir)
	defer dbConn.Close()

	agent, err := db.GetAgent(dbConn, "testuser")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent == nil {
		t.Fatal("expected agent to exist")
	}

	// Verify AAP identity was created
	if agent.AAPGUID == nil {
		t.Fatal("expected agent to have AAP GUID")
	}

	// Verify AAP identity exists on disk
	aapDir := filepath.Join(tmpHome, ".config", "aap", "agents")
	registry, err := aap.NewFileRegistry(aapDir)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	identity, err := registry.Get("testuser")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if identity == nil {
		t.Fatal("expected AAP identity to exist")
	}
	if identity.Record.GUID != *agent.AAPGUID {
		t.Fatalf("AAP GUID mismatch: agent=%s, identity=%s", *agent.AAPGUID, identity.Record.GUID)
	}
}

func TestAgentCreateCreatesAAPIdentity(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

	// Initialize project
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create managed agent
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "agent", "create", "daemonagent", "--driver", "claude"); err != nil {
		t.Fatalf("agent create: %v", err)
	}

	// Verify fray agent was created
	dbConn := openProjectDB(t, projectDir)
	defer dbConn.Close()

	agent, err := db.GetAgent(dbConn, "daemonagent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent == nil {
		t.Fatal("expected agent to exist")
	}
	if !agent.Managed {
		t.Fatal("expected agent to be managed")
	}

	// Verify AAP identity was created
	if agent.AAPGUID == nil {
		t.Fatal("expected agent to have AAP GUID")
	}

	// Verify AAP identity exists on disk
	aapDir := filepath.Join(tmpHome, ".config", "aap", "agents")
	registry, err := aap.NewFileRegistry(aapDir)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	identity, err := registry.Get("daemonagent")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if identity == nil {
		t.Fatal("expected AAP identity to exist")
	}
}

func TestAgentResolveCommand(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

	// Initialize project
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create agent
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "alice", "hello"); err != nil {
		t.Fatalf("new: %v", err)
	}

	// Resolve the agent
	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "agent", "resolve", "@alice")
	if err != nil {
		t.Fatalf("agent resolve: %v", err)
	}

	// Should contain identity info
	if !strings.Contains(output, "alice") {
		t.Fatalf("expected output to contain 'alice', got: %s", output)
	}
	if !strings.Contains(output, "aap-") {
		t.Fatalf("expected output to contain AAP GUID, got: %s", output)
	}
}

func TestAgentIdentityCommand(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

	// Initialize project
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create agent
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "bob", "hello"); err != nil {
		t.Fatalf("new: %v", err)
	}

	// Get identity
	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "agent", "identity", "bob")
	if err != nil {
		t.Fatalf("agent identity: %v", err)
	}

	// Should contain identity info
	if !strings.Contains(output, "bob") {
		t.Fatalf("expected output to contain 'bob', got: %s", output)
	}
	if !strings.Contains(output, "aap-") {
		t.Fatalf("expected output to contain AAP GUID, got: %s", output)
	}
}

func TestMigrateAAPCommand(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

	// Initialize project
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create agent (which already has AAP)
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "legacy", "hello"); err != nil {
		t.Fatalf("new: %v", err)
	}

	// Run migrate (should skip since agent already has AAP)
	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "migrate-aap")
	if err != nil {
		t.Fatalf("migrate-aap: %v", err)
	}

	// Should report skipped
	if !strings.Contains(output, "legacy") && !strings.Contains(output, "Skipped") && !strings.Contains(output, "No agents") {
		t.Fatalf("expected skip or no-op output, got: %s", output)
	}
}

func TestAgentListShowsAAPTag(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

	// Initialize project
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create agent
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "aapuser", "hello"); err != nil {
		t.Fatalf("new: %v", err)
	}

	// List agents
	cmd = NewRootCmd("test")
	output, err := executeCommand(cmd, "agent", "list")
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}

	// Should show AAP tag
	if !strings.Contains(output, "[AAP]") {
		t.Fatalf("expected output to contain [AAP] tag, got: %s", output)
	}
}
