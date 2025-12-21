package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
)

const (
	messagesJSONL = "messages.jsonl"
	historyJSONL  = "history.jsonl"
)

type pruneResult struct {
	Kept           int
	Archived       int
	HistoryPath    string
	ClearedHistory bool
}

func pruneMessages(projectPath string, keep int, pruneAll bool) (pruneResult, error) {
	if keep < 0 {
		return pruneResult{}, fmt.Errorf("invalid --keep value: %d", keep)
	}

	mmDir := resolveMMDir(projectPath)
	messagesPath := filepath.Join(mmDir, messagesJSONL)
	historyPath := filepath.Join(mmDir, historyJSONL)

	if pruneAll {
		if err := os.Remove(historyPath); err != nil && !os.IsNotExist(err) {
			return pruneResult{}, err
		}
	} else if data, err := os.ReadFile(messagesPath); err == nil {
		if strings.TrimSpace(string(data)) != "" {
			if err := appendFile(historyPath, data); err != nil {
				return pruneResult{}, err
			}
		}
	} else if !os.IsNotExist(err) {
		return pruneResult{}, err
	}

	messages, err := db.ReadMessages(projectPath)
	if err != nil {
		return pruneResult{}, err
	}

	kept := messages
	if keep == 0 {
		kept = nil
	} else if len(messages) > keep {
		kept = messages[len(messages)-keep:]
	}

	if err := writeMessages(messagesPath, kept); err != nil {
		return pruneResult{}, err
	}

	archived := 0
	if !pruneAll {
		archived = len(messages)
	}

	return pruneResult{
		Kept:           len(kept),
		Archived:       archived,
		HistoryPath:    historyPath,
		ClearedHistory: pruneAll,
	}, nil
}

func resolveMMDir(projectPath string) string {
	if strings.HasSuffix(projectPath, ".db") {
		return filepath.Dir(projectPath)
	}
	if filepath.Base(projectPath) == ".mm" {
		return projectPath
	}
	return filepath.Join(projectPath, ".mm")
}

func projectRootFromPath(projectPath string) string {
	mmDir := resolveMMDir(projectPath)
	return filepath.Dir(mmDir)
}

func writeMessages(path string, records []db.MessageJSONLRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var builder strings.Builder
	for _, record := range records {
		record.Type = "message"
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}

	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func appendFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}

func checkPruneGuardrails(root string) error {
	if root == "" {
		return fmt.Errorf("unable to determine project root")
	}

	status, err := runGitCommand(root, "status", "--porcelain", ".mm/")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("uncommitted changes in .mm/. Commit first")
	}

	_, err = runGitCommand(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return nil
	}

	aheadStr, err := runGitCommand(root, "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		return err
	}
	behindStr, err := runGitCommand(root, "rev-list", "--count", "HEAD..@{u}")
	if err != nil {
		return err
	}

	ahead, err := strconv.Atoi(strings.TrimSpace(aheadStr))
	if err != nil {
		return err
	}
	behind, err := strconv.Atoi(strings.TrimSpace(behindStr))
	if err != nil {
		return err
	}

	if ahead > 0 || behind > 0 {
		return fmt.Errorf("branch not synced. Push/pull first")
	}

	return nil
}

func runGitCommand(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(output), nil
}
