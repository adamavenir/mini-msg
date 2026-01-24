package command

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

const (
	legacyMessagesFile  = "messages.jsonl"
	legacyAgentsFile    = "agents.jsonl"
	legacyThreadsFile   = "threads.jsonl"
	legacyQuestionsFile = "questions.jsonl"
	agentStateFileName  = "agent-state.jsonl"
	runtimeFileName     = "runtime.jsonl"
)

func migrateMultiMachine(cmd *cobra.Command, project *core.Project) error {
	frayDir := filepath.Dir(project.DBPath)
	if alreadyMigratedMultiMachine(frayDir) {
		fmt.Fprintln(cmd.OutOrStdout(), "Project already migrated to multi-machine storage.")
		return nil
	}

	defaultID := defaultMachineID()
	machineID, err := promptMachineID(project.DBPath, defaultID)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	if machineID == "" {
		return writeCommandError(cmd, fmt.Errorf("machine id required"))
	}

	sharedRoot := filepath.Join(frayDir, "shared")
	machinesRoot := filepath.Join(sharedRoot, "machines")
	machineDir := filepath.Join(machinesRoot, machineID)
	localDir := filepath.Join(frayDir, "local")

	committed := false
	defer func() {
		if err != nil && !committed {
			_ = os.RemoveAll(sharedRoot)
			_ = os.RemoveAll(localDir)
		}
	}()

	if err := os.MkdirAll(machineDir, 0o755); err != nil {
		return writeCommandError(cmd, err)
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return writeCommandError(cmd, err)
	}

	if err := copyFileIfExists(filepath.Join(frayDir, legacyMessagesFile), filepath.Join(machineDir, legacyMessagesFile)); err != nil {
		return writeCommandError(cmd, err)
	}
	if err := copyFileIfExists(filepath.Join(frayDir, legacyThreadsFile), filepath.Join(machineDir, legacyThreadsFile)); err != nil {
		return writeCommandError(cmd, err)
	}
	if err := copyFileIfExists(filepath.Join(frayDir, legacyQuestionsFile), filepath.Join(machineDir, legacyQuestionsFile)); err != nil {
		return writeCommandError(cmd, err)
	}

	runtimeLines, agentStateLines, err := splitAgentsJSONL(filepath.Join(frayDir, legacyAgentsFile))
	if err != nil {
		return writeCommandError(cmd, err)
	}
	if len(agentStateLines) > 0 {
		if err := writeJSONLLines(filepath.Join(machineDir, agentStateFileName), agentStateLines); err != nil {
			return writeCommandError(cmd, err)
		}
	}
	if len(runtimeLines) > 0 {
		if err := writeJSONLLines(filepath.Join(localDir, runtimeFileName), runtimeLines); err != nil {
			return writeCommandError(cmd, err)
		}
	}

	machineRecord := map[string]any{
		"id":         machineID,
		"seq":        0,
		"created_at": time.Now().Unix(),
	}
	data, err := json.Marshal(machineRecord)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "machine-id"), append(data, '\n'), 0o644); err != nil {
		return writeCommandError(cmd, err)
	}

	if err := os.WriteFile(filepath.Join(sharedRoot, ".v2"), []byte(""), 0o644); err != nil {
		return writeCommandError(cmd, err)
	}
	committed = true

	if err := renameIfExists(filepath.Join(frayDir, legacyMessagesFile), filepath.Join(frayDir, legacyMessagesFile+".v1-migrated")); err != nil {
		return writeCommandError(cmd, err)
	}
	if err := renameIfExists(filepath.Join(frayDir, legacyThreadsFile), filepath.Join(frayDir, legacyThreadsFile+".v1-migrated")); err != nil {
		return writeCommandError(cmd, err)
	}
	if err := renameIfExists(filepath.Join(frayDir, legacyQuestionsFile), filepath.Join(frayDir, legacyQuestionsFile+".v1-migrated")); err != nil {
		return writeCommandError(cmd, err)
	}
	if err := renameIfExists(filepath.Join(frayDir, legacyAgentsFile), filepath.Join(frayDir, legacyAgentsFile+".v1-migrated")); err != nil {
		return writeCommandError(cmd, err)
	}

	if _, err := db.UpdateProjectConfig(project.DBPath, db.ProjectConfig{StorageVersion: 2}); err != nil {
		return writeCommandError(cmd, err)
	}

	targetDB, err := sql.Open("sqlite", project.DBPath)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	if _, err := targetDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = targetDB.Close()
		return writeCommandError(cmd, err)
	}
	if _, err := targetDB.Exec("PRAGMA journal_mode = WAL"); err != nil {
		_ = targetDB.Close()
		return writeCommandError(cmd, err)
	}
	if _, err := targetDB.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		_ = targetDB.Close()
		return writeCommandError(cmd, err)
	}

	if err := db.RebuildDatabaseFromJSONL(targetDB, project.DBPath); err != nil {
		_ = targetDB.Close()
		return writeCommandError(cmd, err)
	}
	_ = targetDB.Close()

	fmt.Fprintln(cmd.OutOrStdout(), "✓ Migrated to multi-machine storage")
	return nil
}

func alreadyMigratedMultiMachine(frayDir string) bool {
	if _, err := os.Stat(filepath.Join(frayDir, "shared", ".v2")); err == nil {
		return true
	}
	entries, err := os.ReadDir(frayDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".v1-migrated") {
			return true
		}
	}
	return false
}

func defaultMachineID() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "machine"
	}
	return name
}

func promptMachineID(projectPath, defaultID string) (string, error) {
	if !isTTY(os.Stdin) {
		if db.MachineIDExists(projectPath, defaultID) {
			return "", fmt.Errorf("machine id already exists: %s", defaultID)
		}
		return defaultID, nil
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stdout, "Machine ID for this device? [%s]: ", defaultID)
		text, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			trimmed = defaultID
		}
		if trimmed == "" {
			continue
		}
		if db.MachineIDExists(projectPath, trimmed) {
			fmt.Fprintf(os.Stdout, "Machine ID already exists: %s\n", trimmed)
			continue
		}
		return trimmed, nil
	}
}

func splitAgentsJSONL(path string) ([]string, []string, error) {
	lines, err := readJSONLLines(path)
	if err != nil {
		return nil, nil, err
	}
	if len(lines) == 0 {
		return nil, nil, nil
	}

	agentStateTypes := map[string]bool{
		"ghost_cursor": true,
		"agent_fave":   true,
		"agent_unfave": true,
		"role_hold":    true,
		"role_drop":    true,
		"role_play":    true,
		"role_stop":    true,
	}

	var runtimeLines []string
	var agentStateLines []string
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			runtimeLines = append(runtimeLines, line)
			continue
		}
		if agentStateTypes[envelope.Type] {
			agentStateLines = append(agentStateLines, line)
			continue
		}
		runtimeLines = append(runtimeLines, line)
	}

	return runtimeLines, agentStateLines, nil
}

func writeJSONLLines(filePath string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	if len(lines) == 0 {
		return os.WriteFile(filePath, []byte(""), 0o644)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filePath, []byte(content), 0o644)
}

func copyFileIfExists(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected file but found directory: %s", src)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func renameIfExists(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Rename(src, dst)
}
