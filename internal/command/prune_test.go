package command

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func TestParsePruneProtectionOpts(t *testing.T) {
	defaults := parsePruneProtectionOpts(nil, nil)
	if !defaults.ProtectReplies || !defaults.ProtectFaves || !defaults.ProtectReacts {
		t.Fatalf("expected defaults to protect replies/faves/reacts")
	}
	if defaults.RequireReplies || defaults.RequireFaves || defaults.RequireReacts {
		t.Fatalf("expected defaults to not require attributes")
	}

	opts := parsePruneProtectionOpts([]string{"faves,reacts"}, []string{"replies"})
	if opts.ProtectFaves || opts.ProtectReacts {
		t.Fatalf("expected protections removed for faves/reacts")
	}
	if !opts.ProtectReplies {
		t.Fatalf("expected replies protection to remain")
	}
	if !opts.RequireReplies {
		t.Fatalf("expected replies requirement to be set")
	}
}

func TestResolvePruneTarget(t *testing.T) {
	projectDir, dbConn := initPruneProject(t)
	defer dbConn.Close()

	thread, err := db.CreateThread(dbConn, types.Thread{Name: "design"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	home, name, err := resolvePruneTarget(dbConn, "main")
	if err != nil {
		t.Fatalf("resolve main: %v", err)
	}
	if home != "room" || name != "" {
		t.Fatalf("expected room target, got home=%q name=%q", home, name)
	}

	home, name, err = resolvePruneTarget(dbConn, "design")
	if err != nil {
		t.Fatalf("resolve thread: %v", err)
	}
	if home != thread.GUID || name != thread.Name {
		t.Fatalf("expected thread %s (%s), got home=%q name=%q", thread.Name, thread.GUID, home, name)
	}

	_ = projectDir
}

func TestPruneMessagesKeepsReplyParents(t *testing.T) {
	projectDir, _ := initPruneProject(t)

	parentID := "msg-parent"
	replyID := "msg-reply"

	appendTestMessage(t, projectDir, types.Message{ID: parentID, TS: 1, Home: "room", FromAgent: "alice", Body: "hello", Type: types.MessageTypeAgent})
	appendTestMessage(t, projectDir, types.Message{ID: replyID, TS: 2, Home: "room", FromAgent: "bob", Body: "reply", ReplyTo: &parentID, Type: types.MessageTypeAgent})

	result, err := pruneMessages(projectDir, 1, false, "room", parsePruneProtectionOpts(nil, nil), "")
	if err != nil {
		t.Fatalf("prune messages: %v", err)
	}
	if result.Kept != 2 {
		t.Fatalf("expected kept=2, got %d", result.Kept)
	}

	messages, err := db.ReadMessages(projectDir)
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	foundParent := false
	foundReply := false
	for _, msg := range messages {
		switch msg.ID {
		case parentID:
			foundParent = true
		case replyID:
			foundReply = true
		}
		if msg.MsgType == types.MessageTypeTombstone {
			t.Fatalf("did not expect tombstone when nothing pruned")
		}
	}
	if !foundParent || !foundReply {
		t.Fatalf("expected both parent and reply messages to remain")
	}
}

func TestPruneMessagesWithReaction(t *testing.T) {
	projectDir, _ := initPruneProject(t)

	msg1 := "msg-1"
	msg2 := "msg-2"
	msg3 := "msg-3"

	appendTestMessage(t, projectDir, types.Message{ID: msg1, TS: 1, Home: "room", FromAgent: "alice", Body: "one", Type: types.MessageTypeAgent})
	appendTestMessage(t, projectDir, types.Message{ID: msg2, TS: 2, Home: "room", FromAgent: "bob", Body: "two", Type: types.MessageTypeAgent})
	appendTestMessage(t, projectDir, types.Message{ID: msg3, TS: 3, Home: "room", FromAgent: "cara", Body: "three", Type: types.MessageTypeAgent})

	reactAt := time.Now().Unix()
	if err := db.AppendReaction(projectDir, msg2, "bob", "filed", reactAt); err != nil {
		t.Fatalf("append reaction: %v", err)
	}

	result, err := pruneMessagesWithReaction(projectDir, "room", "filed")
	if err != nil {
		t.Fatalf("prune with reaction: %v", err)
	}
	if result.Kept != 2 || result.Archived != 1 {
		t.Fatalf("expected kept=2 archived=1, got kept=%d archived=%d", result.Kept, result.Archived)
	}

	messages, err := db.ReadMessages(projectDir)
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	found1 := false
	found2 := false
	found3 := false
	foundTombstone := false
	for _, msg := range messages {
		if msg.ID == msg1 {
			found1 = true
		}
		if msg.ID == msg2 {
			found2 = true
		}
		if msg.ID == msg3 {
			found3 = true
		}
		if msg.MsgType == types.MessageTypeTombstone {
			foundTombstone = true
		}
	}
	if !found1 || !found3 {
		t.Fatalf("expected kept messages to remain")
	}
	if found2 {
		t.Fatalf("expected reacted message to be pruned")
	}
	if !foundTombstone {
		t.Fatalf("expected tombstone message")
	}
}

func initPruneProject(t *testing.T) (string, *sql.DB) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

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
	return projectDir, dbConn
}

func appendTestMessage(t *testing.T, projectDir string, message types.Message) {
	t.Helper()
	if message.Home == "" {
		message.Home = "room"
	}
	if err := db.AppendMessage(projectDir, message); err != nil {
		t.Fatalf("append message: %v", err)
	}
	frayDir := filepath.Join(projectDir, ".fray")
	if _, err := os.Stat(frayDir); err != nil {
		t.Fatalf("expected fray dir: %v", err)
	}
}
