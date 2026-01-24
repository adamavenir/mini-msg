package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetStorageVersionUsesV2Sentinel(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray", "shared")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir shared: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frayDir, ".v2"), []byte(""), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if got := GetStorageVersion(projectDir); got != 2 {
		t.Fatalf("expected storage version 2, got %d", got)
	}
}

func TestLegacyWriteBlockedWhenSentinelPresent(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(filepath.Join(frayDir, "shared"), 0o755); err != nil {
		t.Fatalf("mkdir shared: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frayDir, "shared", ".v2"), []byte(""), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frayDir, messagesFile), []byte(""), 0o644); err != nil {
		t.Fatalf("write legacy messages: %v", err)
	}

	if err := ensureLegacyWriteAllowed(projectDir); err == nil {
		t.Fatalf("expected legacy write blocked error")
	}
}
