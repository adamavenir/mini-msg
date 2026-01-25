package command

import (
	"database/sql"
	"os"
	"testing"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func TestAnchorCommandUpdatesThread(t *testing.T) {
	projectDir := setupCommandProject(t)
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "thread", "analysis"); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "post", "--as", "alice", "--thread", "analysis", "anchor body"); err != nil {
		t.Fatalf("post anchor message: %v", err)
	}

	dbConn := openProjectDB(t, projectDir)
	defer dbConn.Close()

	thread := getThreadByName(t, dbConn, "analysis")
	msgID := findThreadMessageByBody(t, dbConn, thread.GUID, "anchor body")

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "anchor", "analysis", msgID); err != nil {
		t.Fatalf("anchor command: %v", err)
	}

	updated, err := db.GetThread(dbConn, thread.GUID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if updated.AnchorMessageGUID == nil || *updated.AnchorMessageGUID != msgID {
		t.Fatalf("expected anchor guid %s, got %v", msgID, updated.AnchorMessageGUID)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "anchor", "analysis", "--hide"); err != nil {
		t.Fatalf("anchor hide: %v", err)
	}

	updated, err = db.GetThread(dbConn, thread.GUID)
	if err != nil {
		t.Fatalf("get thread after hide: %v", err)
	}
	if !updated.AnchorHidden {
		t.Fatalf("expected anchor to be hidden")
	}
}

func TestPinAndUnpinCommands(t *testing.T) {
	projectDir := setupCommandProject(t)
	cmd := NewRootCmd("test")
	if _, err := executeCommand(cmd, "thread", "analysis"); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "post", "--as", "alice", "--thread", "analysis", "pin me"); err != nil {
		t.Fatalf("post message: %v", err)
	}

	dbConn := openProjectDB(t, projectDir)
	defer dbConn.Close()

	thread := getThreadByName(t, dbConn, "analysis")
	msgID := findThreadMessageByBody(t, dbConn, thread.GUID, "pin me")

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "pin", msgID, "--thread", "analysis"); err != nil {
		t.Fatalf("pin command: %v", err)
	}

	pinned, err := db.GetPinnedMessages(dbConn, thread.GUID)
	if err != nil {
		t.Fatalf("get pinned: %v", err)
	}
	if len(pinned) != 1 || pinned[0].ID != msgID {
		t.Fatalf("expected pinned message %s, got %v", msgID, pinned)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "unpin", msgID, "--thread", "analysis"); err != nil {
		t.Fatalf("unpin command: %v", err)
	}

	pinned, err = db.GetPinnedMessages(dbConn, thread.GUID)
	if err != nil {
		t.Fatalf("get pinned after unpin: %v", err)
	}
	if len(pinned) != 0 {
		t.Fatalf("expected no pinned messages, got %d", len(pinned))
	}
}

func setupCommandProject(t *testing.T) string {
	t.Helper()
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

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "alice", "hello"); err != nil {
		t.Fatalf("new command: %v", err)
	}

	return projectDir
}

func getThreadByName(t *testing.T, dbConn *sql.DB, name string) *types.Thread {
	t.Helper()
	thread, err := db.GetThreadByName(dbConn, name, nil)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if thread == nil {
		t.Fatalf("thread not found: %s", name)
	}
	return thread
}

func findThreadMessageByBody(t *testing.T, dbConn *sql.DB, threadGUID, body string) string {
	t.Helper()
	messages, err := db.GetMessages(dbConn, &types.MessageQueryOptions{Home: &threadGUID, IncludeArchived: true})
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	for _, msg := range messages {
		if msg.Body == body {
			return msg.ID
		}
	}
	t.Fatalf("message not found: %s", body)
	return ""
}
