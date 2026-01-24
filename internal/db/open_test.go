package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetJSONLMtimeMultiMachine(t *testing.T) {
	projectDir := t.TempDir()
	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{StorageVersion: 2}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	machineDir := filepath.Join(projectDir, ".fray", "shared", "machines", "laptop")
	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		t.Fatalf("mkdir machine: %v", err)
	}

	older := time.UnixMilli(1000)
	newer := time.UnixMilli(2000)

	messagePath := filepath.Join(machineDir, messagesFile)
	if err := os.WriteFile(messagePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write message: %v", err)
	}
	if err := os.Chtimes(messagePath, older, older); err != nil {
		t.Fatalf("chtimes message: %v", err)
	}

	threadPath := filepath.Join(machineDir, threadsFile)
	if err := os.WriteFile(threadPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write thread: %v", err)
	}
	if err := os.Chtimes(threadPath, newer, newer); err != nil {
		t.Fatalf("chtimes thread: %v", err)
	}

	runtimePath := filepath.Join(projectDir, ".fray", "local", "runtime.jsonl")
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	runtimeTime := time.UnixMilli(3000)
	if err := os.WriteFile(runtimePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	if err := os.Chtimes(runtimePath, runtimeTime, runtimeTime); err != nil {
		t.Fatalf("chtimes runtime: %v", err)
	}

	mtime := getJSONLMtime(projectDir)
	if mtime != runtimeTime.UnixMilli() {
		t.Fatalf("expected mtime %d, got %d", runtimeTime.UnixMilli(), mtime)
	}
}
