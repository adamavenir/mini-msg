package db

import (
	"database/sql"
	"testing"
)

func TestInitSchemaCreatesTables(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	exists, err := SchemaExists(db)
	if err != nil {
		t.Fatalf("schema exists: %v", err)
	}
	if !exists {
		t.Fatal("expected schema to exist")
	}

	rows, err := db.Query(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name LIKE 'fray_%'
		ORDER BY name
	`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		seen[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	for _, table := range []string{"fray_agents", "fray_messages", "fray_linked_projects", "fray_config"} {
		if !seen[table] {
			t.Fatalf("expected table %s", table)
		}
	}
}

func TestDefaultConfigInserted(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	row := db.QueryRow("SELECT value FROM fray_config WHERE key = 'stale_hours'")
	var value string
	if err := row.Scan(&value); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if value != "4" {
		t.Fatalf("expected stale_hours=4, got %s", value)
	}
}

func TestSchemaIdempotent(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema again: %v", err)
	}

	row := db.QueryRow("SELECT COUNT(*) FROM fray_config WHERE key = 'stale_hours'")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count config: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 default config row, got %d", count)
	}
}

func TestAgentColumns(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	rows, err := db.Query("PRAGMA table_info(fray_agents)")
	if err != nil {
		t.Fatalf("table info: %v", err)
	}
	defer rows.Close()

	columns := map[string]sql.NullInt64{}
	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		columns[name] = sql.NullInt64{Int64: int64(pk), Valid: true}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("columns: %v", err)
	}

	for _, name := range []string{"guid", "agent_id", "status", "purpose", "registered_at", "last_seen", "left_at"} {
		if _, ok := columns[name]; !ok {
			t.Fatalf("missing column %s", name)
		}
	}
	if pk, ok := columns["guid"]; !ok || pk.Int64 != 1 {
		t.Fatalf("expected guid to be primary key")
	}
}
