package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func requireSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
}

func strPtr(value string) *string {
	return &value
}

func intPtr(value int64) *int64 {
	return &value
}
