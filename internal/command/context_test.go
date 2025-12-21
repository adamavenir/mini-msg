package command

import (
	"testing"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
)

func TestResolveChannelContextByName(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

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

	channelID := "ch-12345678"
	channelName := "alpha"
	if _, err := db.UpdateProjectConfig(project.DBPath, db.ProjectConfig{
		ChannelID:   channelID,
		ChannelName: channelName,
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	if _, err := core.RegisterChannel(channelID, channelName, projectDir); err != nil {
		t.Fatalf("register channel: %v", err)
	}

	ctx, err := ResolveChannelContext("alpha", projectDir)
	if err != nil {
		t.Fatalf("resolve context: %v", err)
	}
	if ctx.ChannelID != channelID {
		t.Fatalf("expected channel id %s, got %s", channelID, ctx.ChannelID)
	}
	if ctx.Project.Root != projectDir {
		t.Fatalf("expected project root %s, got %s", projectDir, ctx.Project.Root)
	}
}

func TestResolveChannelContextLocal(t *testing.T) {
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

	channelID := "ch-22222222"
	if _, err := db.UpdateProjectConfig(project.DBPath, db.ProjectConfig{
		ChannelID: channelID,
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	ctx, err := ResolveChannelContext("", projectDir)
	if err != nil {
		t.Fatalf("resolve context: %v", err)
	}
	if ctx.ChannelID != channelID {
		t.Fatalf("expected channel id %s, got %s", channelID, ctx.ChannelID)
	}
}
