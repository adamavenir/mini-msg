package command

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
)

func TestFilterCommandLifecycle(t *testing.T) {
	projectDir := t.TempDir()
	project, err := core.InitProject(projectDir, false)
	if err != nil {
		t.Fatalf("init project: %v", err)
	}

	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InitSchema(dbConn); err != nil {
		_ = dbConn.Close()
		t.Fatalf("init schema: %v", err)
	}
	_ = dbConn.Close()

	if _, err := db.UpdateProjectConfig(project.DBPath, db.ProjectConfig{
		Version:     1,
		ChannelID:   "ch-test",
		ChannelName: "test",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("update config: %v", err)
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

	t.Setenv("MM_AGENT_ID", "alice")

	cmd := NewRootCmd("test")
	output, err := executeCommand(cmd, "filter", "show")
	if err != nil {
		t.Fatalf("filter show: %v", err)
	}
	if !strings.Contains(output, "No filter set") {
		t.Fatalf("expected no filter message, got %q", output)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "filter", "set", "--mentions", "claude,pm"); err != nil {
		t.Fatalf("filter set: %v", err)
	}

	cmd = NewRootCmd("test")
	output, err = executeCommand(cmd, "filter", "show")
	if err != nil {
		t.Fatalf("filter show: %v", err)
	}
	if !strings.Contains(output, "Mentions: claude,pm") {
		t.Fatalf("expected mentions to be set, got %q", output)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "filter", "clear"); err != nil {
		t.Fatalf("filter clear: %v", err)
	}

	cmd = NewRootCmd("test")
	output, err = executeCommand(cmd, "filter", "show")
	if err != nil {
		t.Fatalf("filter show: %v", err)
	}
	if !strings.Contains(output, "No filter set") {
		t.Fatalf("expected filter cleared, got %q", output)
	}
}
