package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

func TestChecksumUpdatedOnAppend(t *testing.T) {
	projectDir := t.TempDir()
	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{StorageVersion: 2}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	localDir := filepath.Join(projectDir, ".fray", "local")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "machine-id"), []byte(`{"id":"laptop","seq":0,"created_at":1}`), 0o644); err != nil {
		t.Fatalf("write machine-id: %v", err)
	}

	message := types.Message{ID: "msg-1", TS: 1, FromAgent: "alice", Body: "hi", Mentions: []string{}, Type: types.MessageTypeAgent}
	if err := AppendMessage(projectDir, message); err != nil {
		t.Fatalf("append message: %v", err)
	}

	checksumPath := filepath.Join(projectDir, ".fray", "shared", "checksums.json")
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	var checksums checksumIndex
	if err := json.Unmarshal(data, &checksums); err != nil {
		t.Fatalf("unmarshal checksums: %v", err)
	}
	entry, ok := checksums["laptop"][messagesFile]
	if !ok {
		t.Fatalf("expected checksum entry for laptop/messages.jsonl")
	}
	if entry.SHA256 == "" || entry.Lines != 1 {
		t.Fatalf("unexpected checksum entry: %#v", entry)
	}
}

func TestConcurrentChecksumUpdates(t *testing.T) {
	projectDir := t.TempDir()
	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{StorageVersion: 2}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	machineDir := filepath.Join(projectDir, ".fray", "shared", "machines", "laptop")
	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		t.Fatalf("mkdir machine: %v", err)
	}
	dataPath := filepath.Join(machineDir, messagesFile)
	if err := os.WriteFile(dataPath, []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write data: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := updateChecksum(projectDir, dataPath); err != nil {
				t.Errorf("update checksum: %v", err)
			}
		}()
	}
	wg.Wait()

	checksumPath := filepath.Join(projectDir, ".fray", "shared", "checksums.json")
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	var checksums checksumIndex
	if err := json.Unmarshal(data, &checksums); err != nil {
		t.Fatalf("unmarshal checksums: %v", err)
	}
	if checksums["laptop"][messagesFile].SHA256 == "" {
		t.Fatalf("expected checksum entry populated")
	}
}

func TestChecksumMismatchRecalculated(t *testing.T) {
	projectDir := t.TempDir()
	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{StorageVersion: 2}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	machineDir := filepath.Join(projectDir, ".fray", "shared", "machines", "laptop")
	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		t.Fatalf("mkdir machine: %v", err)
	}
	dataPath := filepath.Join(machineDir, messagesFile)
	if err := os.WriteFile(dataPath, []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write data: %v", err)
	}

	sum, lines, mtime, err := computeChecksum(dataPath)
	if err != nil {
		t.Fatalf("compute checksum: %v", err)
	}

	checksumPath := filepath.Join(projectDir, ".fray", "shared", "checksums.json")
	checksums := checksumIndex{
		"laptop": {
			messagesFile: {SHA256: "bad", Lines: 1, Mtime: mtime},
		},
	}
	payload, _ := json.Marshal(checksums)
	if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
		t.Fatalf("mkdir checksums: %v", err)
	}
	if err := os.WriteFile(checksumPath, payload, 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	if err := validateChecksums(projectDir); err != nil {
		t.Fatalf("validate: %v", err)
	}

	data, err := os.ReadFile(checksumPath)
	if err != nil {
		t.Fatalf("read checksums: %v", err)
	}
	var updated checksumIndex
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("unmarshal checksums: %v", err)
	}
	entry := updated["laptop"][messagesFile]
	if entry.SHA256 != sum || entry.Lines != lines {
		t.Fatalf("expected updated checksum, got %#v", entry)
	}
}

func TestValidateChecksumsMissingFile(t *testing.T) {
	projectDir := t.TempDir()
	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{StorageVersion: 2}); err != nil {
		t.Fatalf("update config: %v", err)
	}
	if err := validateChecksums(projectDir); err != nil {
		t.Fatalf("validate checksums: %v", err)
	}
}
