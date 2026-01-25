package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	messagesFile      = "messages.jsonl"
	agentsFile        = "agents.jsonl"
	agentStateFile    = "agent-state.jsonl"
	questionsFile     = "questions.jsonl"
	threadsFile       = "threads.jsonl"
	permissionsFile   = "permissions.jsonl"
	projectConfigFile = "fray-config.json"
	runtimeFile       = "runtime.jsonl"
)

func GetStorageVersion(projectPath string) int {
	config, err := ReadProjectConfig(projectPath)
	if err != nil || config == nil || config.StorageVersion == 0 {
		if hasV2Sentinel(projectPath) {
			return 2
		}
		return 1
	}
	if config.StorageVersion < 2 && hasV2Sentinel(projectPath) {
		return 2
	}
	return config.StorageVersion
}

func hasV2Sentinel(projectPath string) bool {
	frayDir := resolveFrayDir(projectPath)
	_, err := os.Stat(filepath.Join(frayDir, "shared", ".v2"))
	return err == nil
}

func legacyWriteBlocked(projectPath string) (bool, []string, error) {
	if !hasV2Sentinel(projectPath) {
		return false, nil, nil
	}
	frayDir := resolveFrayDir(projectPath)
	legacyFiles := []string{messagesFile, threadsFile, questionsFile, agentsFile}
	var present []string
	for _, name := range legacyFiles {
		path := filepath.Join(frayDir, name)
		if _, err := os.Stat(path); err == nil {
			present = append(present, name)
		} else if !os.IsNotExist(err) {
			return false, nil, err
		}
	}
	if len(present) == 0 {
		return false, nil, nil
	}
	return true, present, nil
}

func ensureLegacyWriteAllowed(projectPath string) error {
	blocked, files, err := legacyWriteBlocked(projectPath)
	if err != nil {
		return err
	}
	if !blocked {
		return nil
	}
	return fmt.Errorf("legacy JSONL files detected in multi-machine project (%s). Remove legacy files or re-run `fray migrate --multi-machine`", strings.Join(files, ", "))
}

// IsMultiMachineMode reports whether storage_version >= 2.
func IsMultiMachineMode(projectPath string) bool {
	return GetStorageVersion(projectPath) >= 2
}

// GetLocalMachineID returns the ID from local/machine-id or empty string.
func GetLocalMachineID(projectPath string) string {
	frayDir := resolveFrayDir(projectPath)
	path := filepath.Join(frayDir, "local", "machine-id")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var record machineIDFile
	if err := json.Unmarshal(data, &record); err != nil {
		return ""
	}
	return record.ID
}

// GetSharedMachinesDirs returns paths to all shared machine directories.
func GetSharedMachinesDirs(projectPath string) []string {
	frayDir := resolveFrayDir(projectPath)
	machinesRoot := filepath.Join(frayDir, "shared", "machines")
	entries, err := os.ReadDir(machinesRoot)
	if err != nil {
		return nil
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(machinesRoot, entry.Name()))
		}
	}
	sort.Strings(dirs)
	return dirs
}

// MachineIDExists reports whether a shared machine directory exists for the ID.
func MachineIDExists(projectPath, machineID string) bool {
	if machineID == "" {
		return false
	}
	frayDir := resolveFrayDir(projectPath)
	path := filepath.Join(frayDir, "shared", "machines", machineID)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetLocalMachineDir returns the shared directory for the local machine.
func GetLocalMachineDir(projectPath string) string {
	localID := GetLocalMachineID(projectPath)
	if localID == "" {
		return ""
	}
	frayDir := resolveFrayDir(projectPath)
	return filepath.Join(frayDir, "shared", "machines", localID)
}

// GetLocalRuntimePath returns the local runtime.jsonl path.
func GetLocalRuntimePath(projectPath string) string {
	frayDir := resolveFrayDir(projectPath)
	return filepath.Join(frayDir, "local", "runtime.jsonl")
}

// GetNextSequence returns the next per-machine sequence value.
func GetNextSequence(projectPath string) (int64, error) {
	frayDir := resolveFrayDir(projectPath)
	localDir := filepath.Join(frayDir, "local")
	if err := ensureDir(localDir); err != nil {
		return 0, err
	}

	path := filepath.Join(localDir, "machine-id")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return 0, err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	if _, err := file.Seek(0, 0); err != nil {
		return 0, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return 0, err
	}

	record, seqPresent, parsed := parseMachineIDFile(data)
	if !parsed {
		record.ID = extractMachineIDFromRaw(data)
	}
	if record.ID == "" {
		return 0, fmt.Errorf("machine-id missing id")
	}

	seq := record.Seq
	if !seqPresent {
		recovered, err := recoverSequenceFromJSONL(projectPath, record.ID)
		if err != nil {
			return 0, err
		}
		seq = recovered
	}

	next := seq + 1
	record.Seq = next
	if record.CreatedAt == 0 {
		record.CreatedAt = time.Now().Unix()
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return 0, err
	}
	encoded = append(encoded, '\n')

	if _, err := file.Seek(0, 0); err != nil {
		return 0, err
	}
	if err := file.Truncate(0); err != nil {
		return 0, err
	}
	if _, err := file.Write(encoded); err != nil {
		return 0, err
	}
	if err := file.Sync(); err != nil {
		return 0, err
	}

	return next, nil
}

func parseMachineIDFile(data []byte) (machineIDFile, bool, bool) {
	var record machineIDFile
	if len(bytes.TrimSpace(data)) == 0 {
		return record, false, false
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return record, false, false
	}

	if rawID, ok := raw["id"]; ok {
		_ = json.Unmarshal(rawID, &record.ID)
	}
	if rawCreated, ok := raw["created_at"]; ok {
		_ = json.Unmarshal(rawCreated, &record.CreatedAt)
	}

	seqPresent := false
	if rawSeq, ok := raw["seq"]; ok {
		var seq int64
		if err := json.Unmarshal(rawSeq, &seq); err == nil {
			seqPresent = true
			record.Seq = seq
		}
	}

	return record, seqPresent, true
}

func extractMachineIDFromRaw(data []byte) string {
	raw := string(data)
	idx := strings.Index(raw, `"id"`)
	if idx == -1 {
		return ""
	}
	raw = raw[idx+len(`"id"`):]
	colon := strings.Index(raw, ":")
	if colon == -1 {
		return ""
	}
	raw = strings.TrimLeft(raw[colon+1:], " \t\r\n")
	if !strings.HasPrefix(raw, `"`) {
		return ""
	}
	raw = raw[1:]
	end := strings.Index(raw, `"`)
	if end == -1 {
		return ""
	}
	return raw[:end]
}

func recoverSequenceFromJSONL(projectPath, machineID string) (int64, error) {
	frayDir := resolveFrayDir(projectPath)
	var dirs []string
	if machineID != "" {
		dirs = []string{filepath.Join(frayDir, "shared", "machines", machineID)}
	} else {
		dirs = GetSharedMachinesDirs(projectPath)
	}

	var maxSeq int64
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			lines, err := readJSONLLines(filepath.Join(dir, entry.Name()))
			if err != nil {
				return 0, err
			}
			for _, line := range lines {
				var envelope struct {
					Seq *int64 `json:"seq"`
				}
				if err := json.Unmarshal([]byte(line), &envelope); err != nil {
					continue
				}
				if envelope.Seq != nil && *envelope.Seq > maxSeq {
					maxSeq = *envelope.Seq
				}
			}
		}
	}

	return maxSeq, nil
}
