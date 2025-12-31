package command

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
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

func TestEditRequiresReasonAndCreatesEvent(t *testing.T) {
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

	dbConn := openProjectDB(t, projectDir)
	msgID := findRoomMessageByBody(t, dbConn, "ping")
	_ = dbConn.Close()

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "edit", msgID, "pong", "--as", "alice"); err == nil {
		t.Fatalf("expected edit to require -m")
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "edit", msgID, "pong", "--as", "alice", "-m", "fix typo"); err != nil {
		t.Fatalf("edit command: %v", err)
	}

	dbConn = openProjectDB(t, projectDir)
	defer dbConn.Close()
	messages, err := db.GetMessages(dbConn, &types.MessageQueryOptions{IncludeArchived: true})
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}

	var event *types.Message
	for _, msg := range messages {
		if msg.Type != types.MessageTypeEvent {
			continue
		}
		if msg.References != nil && *msg.References == msgID {
			event = &msg
			break
		}
	}
	if event == nil {
		t.Fatalf("expected edit event message")
	}
	if event.FromAgent != "alice" {
		t.Fatalf("expected event from alice, got %s", event.FromAgent)
	}
	if !strings.Contains(event.Body, "edited #") || !strings.Contains(event.Body, "fix typo") {
		t.Fatalf("unexpected event body: %q", event.Body)
	}
	if event.Home != "room" {
		t.Fatalf("expected event in room, got %q", event.Home)
	}
}

func TestQuestionLifecycleFlow(t *testing.T) {
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
		t.Fatalf("new alice: %v", err)
	}
	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "new", "bob", "hello"); err != nil {
		t.Fatalf("new bob: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "ask", "target market?", "--as", "alice", "--to", "bob"); err != nil {
		t.Fatalf("ask command: %v", err)
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

	questions, err := db.GetQuestions(dbConn, &types.QuestionQueryOptions{})
	if err != nil {
		t.Fatalf("get questions: %v", err)
	}
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "post", "--as", "bob", "--answer", questions[0].GUID, "Small B2B SaaS"); err != nil {
		t.Fatalf("answer command: %v", err)
	}

	updated, err := db.GetQuestion(dbConn, questions[0].GUID)
	if err != nil {
		t.Fatalf("get question: %v", err)
	}
	if updated == nil || updated.Status != types.QuestionStatusAnswered {
		t.Fatalf("expected answered status, got %v", updated)
	}
}

func TestThreadCommandFlow(t *testing.T) {
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
		t.Fatalf("new alice: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "thread", "new", "analysis"); err != nil {
		t.Fatalf("thread new: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "post", "--as", "alice", "room message"); err != nil {
		t.Fatalf("post command: %v", err)
	}

	dbConn := openProjectDB(t, projectDir)
	roomMsgID := findRoomMessageByBody(t, dbConn, "room message")
	_ = dbConn.Close()

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "thread", "add", "analysis", roomMsgID, "--as", "alice"); err != nil {
		t.Fatalf("thread add: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "thread", "subscribe", "analysis", "--agent", "alice"); err != nil {
		t.Fatalf("thread subscribe: %v", err)
	}

	dbConn = openProjectDB(t, projectDir)
	defer dbConn.Close()
	thread, err := db.GetThreadByName(dbConn, "analysis", nil)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if thread == nil {
		t.Fatal("expected thread")
	}

	agentID := "alice"
	threads, err := db.GetThreads(dbConn, &types.ThreadQueryOptions{SubscribedAgent: &agentID})
	if err != nil {
		t.Fatalf("get subscribed threads: %v", err)
	}
	if len(threads) != 1 || threads[0].GUID != thread.GUID {
		t.Fatalf("expected subscription to analysis thread")
	}

	inThread, err := db.IsMessageInThread(dbConn, thread.GUID, roomMsgID)
	if err != nil {
		t.Fatalf("message in thread: %v", err)
	}
	if !inThread {
		t.Fatalf("expected room message added to thread")
	}
}

func TestCrossThreadReplyAutoAdd(t *testing.T) {
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
		t.Fatalf("new alice: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "thread", "new", "analysis"); err != nil {
		t.Fatalf("thread new: %v", err)
	}

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "post", "--as", "alice", "room message"); err != nil {
		t.Fatalf("post command: %v", err)
	}

	dbConn := openProjectDB(t, projectDir)
	roomMsgID := findRoomMessageByBody(t, dbConn, "room message")
	_ = dbConn.Close()

	cmd = NewRootCmd("test")
	if _, err := executeCommand(cmd, "post", "--as", "alice", "--thread", "analysis", "--reply-to", roomMsgID, "This is a longer reply in the thread"); err != nil {
		t.Fatalf("thread reply: %v", err)
	}

	dbConn = openProjectDB(t, projectDir)
	defer dbConn.Close()
	thread, err := db.GetThreadByName(dbConn, "analysis", nil)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if thread == nil {
		t.Fatal("expected thread")
	}

	inThread, err := db.IsMessageInThread(dbConn, thread.GUID, roomMsgID)
	if err != nil {
		t.Fatalf("message in thread: %v", err)
	}
	if !inThread {
		t.Fatalf("expected reply to add parent message to thread")
	}
}

func openProjectDB(t *testing.T, projectDir string) *sql.DB {
	t.Helper()

	project, err := core.DiscoverProject(projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}

	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return dbConn
}

func findRoomMessageByBody(t *testing.T, dbConn *sql.DB, body string) string {
	t.Helper()

	home := "room"
	messages, err := db.GetMessages(dbConn, &types.MessageQueryOptions{Home: &home, IncludeArchived: true})
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
