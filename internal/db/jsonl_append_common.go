package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func appendJSONLine(filePath string, record any) error {
	if err := ensureDir(filepath.Dir(filePath)); err != nil {
		return err
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return atomicAppend(filePath, data)
}

func appendSharedJSONLine(projectPath, filePath string, record any) error {
	if err := appendJSONLine(filePath, record); err != nil {
		return err
	}
	if IsMultiMachineMode(projectPath) {
		if err := updateChecksum(projectPath, filePath); err != nil {
			return err
		}
	}
	return nil
}

func atomicAppend(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}

	return f.Sync()
}

func sharedMachinePath(projectPath, fileName string) (string, error) {
	if !IsMultiMachineMode(projectPath) {
		frayDir := resolveFrayDir(projectPath)
		return filepath.Join(frayDir, fileName), nil
	}
	dir := GetLocalMachineDir(projectPath)
	if dir == "" {
		return "", fmt.Errorf("local machine id not set")
	}
	return filepath.Join(dir, fileName), nil
}

func agentStatePath(projectPath string) (string, error) {
	if !IsMultiMachineMode(projectPath) {
		frayDir := resolveFrayDir(projectPath)
		return filepath.Join(frayDir, agentsFile), nil
	}
	return sharedMachinePath(projectPath, agentStateFile)
}

func runtimePath(projectPath string) string {
	if IsMultiMachineMode(projectPath) {
		return GetLocalRuntimePath(projectPath)
	}
	frayDir := resolveFrayDir(projectPath)
	return filepath.Join(frayDir, agentsFile)
}
