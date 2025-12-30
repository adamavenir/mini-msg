package command

import (
	"os"
	"testing"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func TestRenameCommandRenamesAgent(t *testing.T) {
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
	agentID := "alice"
	guid, err := core.GenerateGUID("usr")
	if err != nil {
		t.Fatalf("generate guid: %v", err)
	}
	if err := db.CreateAgent(dbConn, types.Agent{
		GUID:         guid,
		AgentID:      agentID,
		RegisteredAt: now,
		LastSeen:     now,
	}); err != nil {
		_ = dbConn.Close()
		t.Fatalf("create agent: %v", err)
	}

	posted, err := db.CreateMessage(dbConn, types.Message{
		TS:        now,
		FromAgent: agentID,
		Body:      "hi @alice",
		Mentions:  []string{agentID},
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
	if _, err := executeCommand(cmd, "rename", "alice", "bob"); err != nil {
		t.Fatalf("rename command: %v", err)
	}

	dbConn, err = db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer dbConn.Close()

	if agent, err := db.GetAgent(dbConn, "bob"); err != nil || agent == nil {
		t.Fatalf("expected bob to exist")
	}
	if agent, err := db.GetAgent(dbConn, "alice"); err != nil {
		t.Fatalf("get agent: %v", err)
	} else if agent != nil {
		t.Fatalf("expected alice to be renamed")
	}

	updated, err := db.GetMessage(dbConn, posted.ID)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if updated == nil {
		t.Fatal("expected message to exist")
	}
	found := false
	for _, mention := range updated.Mentions {
		if mention == "bob" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected mentions to include bob, got %#v", updated.Mentions)
	}
}
