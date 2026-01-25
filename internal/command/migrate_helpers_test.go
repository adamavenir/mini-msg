package command

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestResolveReplyToMapsLegacyIDs(t *testing.T) {
	idToGuid := map[int64]string{42: "msg-42"}

	resolved := resolveReplyTo(int64(42), false, idToGuid)
	if resolved == nil || *resolved != "msg-42" {
		t.Fatalf("expected msg-42, got %v", resolved)
	}

	resolved = resolveReplyTo("42", false, idToGuid)
	if resolved == nil || *resolved != "msg-42" {
		t.Fatalf("expected msg-42 from string, got %v", resolved)
	}

	resolved = resolveReplyTo("msg-abc", true, idToGuid)
	if resolved == nil || *resolved != "msg-abc" {
		t.Fatalf("expected msg-abc, got %v", resolved)
	}

	resolved = resolveReplyTo(int64(99), false, idToGuid)
	if resolved != nil {
		t.Fatalf("expected nil for unknown id, got %v", *resolved)
	}
}

func TestLoadReadReceiptsHandlesLegacyIDs(t *testing.T) {
	dbConn := openLegacyDB(t)
	defer dbConn.Close()

	_, err := dbConn.Exec(`CREATE TABLE fray_read_receipts (message_id INTEGER, agent_prefix TEXT, read_at INTEGER)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = dbConn.Exec(`INSERT INTO fray_read_receipts (message_id, agent_prefix, read_at) VALUES (1, 'al', 123)`)
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}

	receipts, err := loadReadReceipts(dbConn)
	if err != nil {
		t.Fatalf("load read receipts: %v", err)
	}
	if len(receipts) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(receipts))
	}
	if receipts[0].MessageID == nil || *receipts[0].MessageID != 1 {
		t.Fatalf("expected message id 1, got %v", receipts[0].MessageID)
	}
	if receipts[0].MessageGUID != nil {
		t.Fatalf("expected nil message guid, got %v", receipts[0].MessageGUID)
	}
}

func TestLoadReadReceiptsHandlesGUIDs(t *testing.T) {
	dbConn := openLegacyDB(t)
	defer dbConn.Close()

	_, err := dbConn.Exec(`CREATE TABLE fray_read_receipts (message_guid TEXT, agent_prefix TEXT, read_at INTEGER)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = dbConn.Exec(`INSERT INTO fray_read_receipts (message_guid, agent_prefix, read_at) VALUES ('msg-abc', 'al', 123)`)
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}

	receipts, err := loadReadReceipts(dbConn)
	if err != nil {
		t.Fatalf("load read receipts: %v", err)
	}
	if len(receipts) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(receipts))
	}
	if receipts[0].MessageGUID == nil || *receipts[0].MessageGUID != "msg-abc" {
		t.Fatalf("expected message guid msg-abc, got %v", receipts[0].MessageGUID)
	}
}

func openLegacyDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	dbConn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return dbConn
}
