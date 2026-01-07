package command

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func TestGetMessageShowsFullMessage(t *testing.T) {
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

	dbConn, err = db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	now := time.Now().Unix()
	if err := db.CreateAgent(dbConn, types.Agent{
		GUID:         "usr-1234",
		AgentID:      "alice",
		RegisteredAt: now,
		LastSeen:     now,
	}); err != nil {
		_ = dbConn.Close()
		t.Fatalf("create agent: %v", err)
	}

	body := "line one\nline two\nline three"
	posted, err := db.CreateMessage(dbConn, types.Message{
		TS:        now,
		FromAgent: "alice",
		Body:      body,
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		_ = dbConn.Close()
		t.Fatalf("create message: %v", err)
	}
	_ = dbConn.Close()

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
	output, err := executeCommand(cmd, "get", posted.ID)
	if err != nil {
		t.Fatalf("get message command: %v", err)
	}

	if !strings.Contains(output, posted.ID) {
		t.Fatalf("expected message ID in output, got %q", output)
	}
	if !strings.Contains(output, body) {
		t.Fatalf("expected full body in output, got %q", output)
	}
}
