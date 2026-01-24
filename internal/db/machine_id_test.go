package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMachineIDExists(t *testing.T) {
	projectDir := t.TempDir()
	machineDir := filepath.Join(projectDir, ".fray", "shared", "machines", "laptop")
	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		t.Fatalf("mkdir machine dir: %v", err)
	}

	if !MachineIDExists(projectDir, "laptop") {
		t.Fatalf("expected machine id to exist")
	}
	if MachineIDExists(projectDir, "server") {
		t.Fatalf("expected machine id to be free")
	}
}
