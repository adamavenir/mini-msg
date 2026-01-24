package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

func TestCollisionLogRecordsMessageGUID(t *testing.T) {
	projectDir := t.TempDir()
	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{StorageVersion: 2}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	machines := []string{"alpha", "beta"}
	for _, machine := range machines {
		dir := filepath.Join(projectDir, ".fray", "shared", "machines", machine)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir machine: %v", err)
		}
		msg := MessageJSONLRecord{
			Type:      "message",
			ID:        "msg-collide",
			FromAgent: machine,
			Body:      machine,
			Mentions:  []string{},
			MsgType:   types.MessageTypeAgent,
			TS:        100,
		}
		data, _ := json.Marshal(msg)
		if err := os.WriteFile(filepath.Join(dir, messagesFile), append(data, '\n'), 0o644); err != nil {
			t.Fatalf("write messages: %v", err)
		}
	}

	dbConn := openTestDB(t)
	if err := RebuildDatabaseFromJSONL(dbConn, projectDir); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	logData, err := ReadCollisionLog(projectDir)
	if err != nil {
		t.Fatalf("read collision log: %v", err)
	}
	if logData == nil || len(logData.Collisions) != 1 {
		t.Fatalf("expected 1 collision, got %#v", logData)
	}
	collision := logData.Collisions[0]
	if collision.Type != "message" || collision.GUID != "msg-collide" {
		t.Fatalf("unexpected collision entry: %#v", collision)
	}
	if len(collision.Entries) != 2 {
		t.Fatalf("expected 2 collision entries, got %d", len(collision.Entries))
	}
	seen := make(map[string]bool)
	for _, entry := range collision.Entries {
		seen[entry.Machine] = true
	}
	for _, machine := range machines {
		if !seen[machine] {
			t.Fatalf("missing collision entry for %s", machine)
		}
	}

	if err := ClearCollisionLog(projectDir); err != nil {
		t.Fatalf("clear collision log: %v", err)
	}
	logData, err = ReadCollisionLog(projectDir)
	if err != nil {
		t.Fatalf("read collision log: %v", err)
	}
	if logData == nil || len(logData.Collisions) != 0 {
		t.Fatalf("expected empty collision log after clear, got %#v", logData)
	}
}
