package command

import (
	"os"
	"testing"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
)

func TestInitNewPostFlow(t *testing.T) {
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

	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "init", "--defaults"); err != nil {
		t.Fatalf("init command: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "alice", "hello"); err != nil {
		t.Fatalf("new command: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "post", "--as", "alice", "ping"); err != nil {
		t.Fatalf("post command: %v", err)
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

	if err := db.InitSchema(dbConn); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	agent, err := db.GetAgent(dbConn, "alice")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent == nil {
		t.Fatal("expected agent to exist")
	}

	messages, err := db.GetMessages(dbConn, &types.MessageQueryOptions{IncludeArchived: true})
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}
}
