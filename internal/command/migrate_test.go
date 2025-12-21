package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adamavenir/mini-msg/internal/core"
)

func TestMigrateCommandFailsWhenConfigExists(t *testing.T) {
	projectDir := t.TempDir()
	project, err := core.InitProject(projectDir, false)
	if err != nil {
		t.Fatalf("init project: %v", err)
	}

	if err := os.WriteFile(project.DBPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	configPath := filepath.Join(filepath.Dir(project.DBPath), "mm-config.json")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	cmd := NewRootCmd("test")
	output, err := executeCommand(cmd, "migrate")
	if err == nil {
		t.Fatalf("expected error, got output: %q", output)
	}
	if !strings.Contains(output, "mm-config.json already exists") {
		t.Fatalf("unexpected output: %q", output)
	}
}
