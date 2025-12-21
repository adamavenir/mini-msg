package chat

import (
	"database/sql"
	"testing"

	"github.com/adamavenir/mini-msg/internal/db"
	_ "modernc.org/sqlite"
)

func TestResolveReplyReference(t *testing.T) {
	dbConn := openChatDB(t)
	seedMessage(t, dbConn, "msg-abc11111", 100, "alice", "one")
	seedMessage(t, dbConn, "msg-xyz22222", 200, "bob", "two")

	resolution, err := ResolveReplyReference(dbConn, "#abc hello")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Kind != ReplyResolved {
		t.Fatalf("expected resolved, got %s", resolution.Kind)
	}
	if resolution.ReplyTo != "msg-abc11111" {
		t.Fatalf("unexpected reply_to %s", resolution.ReplyTo)
	}
	if resolution.Body != "hello" {
		t.Fatalf("unexpected body %q", resolution.Body)
	}

	resolution, err = ResolveReplyReference(dbConn, "no reply")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Kind != ReplyNone {
		t.Fatalf("expected none, got %s", resolution.Kind)
	}
}

func TestResolveReplyReferenceAmbiguous(t *testing.T) {
	dbConn := openChatDB(t)
	seedMessage(t, dbConn, "msg-abc11111", 100, "alice", "one")
	seedMessage(t, dbConn, "msg-abc22222", 200, "bob", "two")

	resolution, err := ResolveReplyReference(dbConn, "#abc hello")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Kind != ReplyAmbiguous {
		t.Fatalf("expected ambiguous, got %s", resolution.Kind)
	}
	if len(resolution.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(resolution.Matches))
	}
}

func openChatDB(t *testing.T) *sql.DB {
	t.Helper()
	path := t.TempDir() + "/test.db"
	dbConn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	if err := db.InitSchema(dbConn); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return dbConn
}

func seedMessage(t *testing.T, dbConn *sql.DB, guid string, ts int64, fromAgent, body string) {
	t.Helper()
	_, err := dbConn.Exec(`
		INSERT INTO mm_messages (
			guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, guid, ts, nil, fromAgent, body, "[]", "agent", nil, nil, nil)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
}
