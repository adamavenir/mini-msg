package command

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/spf13/cobra"
)

// NewPruneCmd creates the prune command.
func NewPruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Archive old messages with cold storage guardrails",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			keep, _ := cmd.Flags().GetInt("keep")
			pruneAll, _ := cmd.Flags().GetBool("all")

			if keep < 0 {
				return writeCommandError(cmd, fmt.Errorf("invalid --keep value: %d", keep))
			}

			if err := checkPruneGuardrails(ctx.Project.Root); err != nil {
				return writeCommandError(cmd, err)
			}

			result, err := pruneMessages(ctx.Project.DBPath, keep, pruneAll)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := db.RebuildDatabaseFromJSONL(ctx.DB, ctx.Project.DBPath); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"kept":     result.Kept,
					"archived": result.Archived,
				}
				if result.ClearedHistory {
					payload["history"] = nil
				} else {
					payload["history"] = result.HistoryPath
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if result.ClearedHistory {
				fmt.Fprintf(out, "Pruned to last %d messages. history.jsonl cleared.\n", result.Kept)
				return nil
			}
			fmt.Fprintf(out, "Pruned to last %d messages. Archived to history.jsonl\n", result.Kept)
			return nil
		},
	}

	cmd.Flags().Int("keep", 20, "number of recent messages to keep")
	cmd.Flags().Bool("all", false, "delete history.jsonl before pruning")
	return cmd
}

type pruneResult struct {
	Kept           int
	Archived       int
	HistoryPath    string
	ClearedHistory bool
}

func pruneMessages(projectPath string, keep int, pruneAll bool) (pruneResult, error) {
	mmDir := resolveMMDir(projectPath)
	messagesPath := filepath.Join(mmDir, "messages.jsonl")
	historyPath := filepath.Join(mmDir, "history.jsonl")

	if pruneAll {
		keep = 0
	}

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

	if keep > 0 && len(kept) > 0 && len(kept) < len(messages) {
		keepIDs := make(map[string]struct{}, len(kept))
		byID := make(map[string]db.MessageJSONLRecord, len(messages))
		for _, msg := range messages {
			byID[msg.ID] = msg
		}
		for _, msg := range kept {
			keepIDs[msg.ID] = struct{}{}
		}

		for _, msg := range kept {
			parentID := msg.ReplyTo
			for parentID != nil && *parentID != "" {
				id := *parentID
				if _, ok := keepIDs[id]; ok {
					parent, ok := byID[id]
					if !ok {
						break
					}
					parentID = parent.ReplyTo
					continue
				}
				keepIDs[id] = struct{}{}
				parent, ok := byID[id]
				if !ok {
					break
				}
				parentID = parent.ReplyTo
			}
		}

		if len(keepIDs) != len(kept) {
			filtered := make([]db.MessageJSONLRecord, 0, len(keepIDs))
			for _, msg := range messages {
				if _, ok := keepIDs[msg.ID]; ok {
					filtered = append(filtered, msg)
				}
			}
			kept = filtered
		}
	}

	if err := writeMessages(messagesPath, kept); err != nil {
		return pruneResult{}, err
	}

	archived := 0
	if !pruneAll {
		archived = len(messages)
	}

	return pruneResult{Kept: len(kept), Archived: archived, HistoryPath: historyPath, ClearedHistory: pruneAll}, nil
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
