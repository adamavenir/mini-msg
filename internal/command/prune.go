package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adamavenir/fray/internal/db"
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
	frayDir := resolveFrayDir(projectPath)
	messagesPath := filepath.Join(frayDir, "messages.jsonl")
	historyPath := filepath.Join(frayDir, "history.jsonl")

	if pruneAll {
		keep = 0
	}

	// Handle history archival
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

	// Read all messages
	messages, err := db.ReadMessages(projectPath)
	if err != nil {
		return pruneResult{}, err
	}

	// Collect IDs that must be preserved for integrity
	requiredIDs, err := collectRequiredMessageIDs(projectPath)
	if err != nil {
		return pruneResult{}, err
	}

	kept := messages
	if pruneAll || keep == 0 {
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

		// Add required IDs for integrity
		for id := range requiredIDs {
			keepIDs[id] = struct{}{}
		}

		// Follow reply chains to preserve parents
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

		// Also follow reply chains for newly-required messages
		for id := range requiredIDs {
			msg, ok := byID[id]
			if !ok {
				continue
			}
			parentID := msg.ReplyTo
			for parentID != nil && *parentID != "" {
				pid := *parentID
				if _, ok := keepIDs[pid]; ok {
					parent, ok := byID[pid]
					if !ok {
						break
					}
					parentID = parent.ReplyTo
					continue
				}
				keepIDs[pid] = struct{}{}
				parent, ok := byID[pid]
				if !ok {
					break
				}
				parentID = parent.ReplyTo
			}
		}

		// Rebuild kept messages preserving order
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

	// Build set of kept message IDs for event filtering
	keptIDSet := make(map[string]struct{}, len(kept))
	for _, msg := range kept {
		keptIDSet[msg.ID] = struct{}{}
	}

	// Write messages with their associated events
	if err := writeMessagesWithEvents(messagesPath, kept, keptIDSet); err != nil {
		return pruneResult{}, err
	}

	archived := 0
	if !pruneAll {
		archived = len(messages)
	}

	return pruneResult{Kept: len(kept), Archived: archived, HistoryPath: historyPath, ClearedHistory: pruneAll}, nil
}

// collectRequiredMessageIDs gathers message IDs that must be preserved for data integrity.
func collectRequiredMessageIDs(projectPath string) (map[string]struct{}, error) {
	required := make(map[string]struct{})
	frayDir := resolveFrayDir(projectPath)

	// Read threads for anchor messages
	threads, _, _, err := db.ReadThreads(projectPath)
	if err != nil {
		return nil, err
	}
	for _, thread := range threads {
		if thread.AnchorMessageGUID != nil && *thread.AnchorMessageGUID != "" {
			required[*thread.AnchorMessageGUID] = struct{}{}
		}
	}

	// Read message pins
	pinEvents, err := db.ReadMessagePins(projectPath)
	if err != nil {
		return nil, err
	}
	// Track current pinned state
	pinnedMessages := make(map[string]struct{})
	for _, event := range pinEvents {
		key := event.MessageGUID + "|" + event.ThreadGUID
		if event.Type == "message_pin" {
			pinnedMessages[key] = struct{}{}
		} else if event.Type == "message_unpin" {
			delete(pinnedMessages, key)
		}
	}
	for key := range pinnedMessages {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) > 0 {
			required[parts[0]] = struct{}{}
		}
	}

	// Read questions for message references
	questions, err := db.ReadQuestions(projectPath)
	if err != nil {
		return nil, err
	}
	for _, q := range questions {
		if q.AskedIn != nil && *q.AskedIn != "" {
			required[*q.AskedIn] = struct{}{}
		}
		if q.AnsweredIn != nil && *q.AnsweredIn != "" {
			required[*q.AnsweredIn] = struct{}{}
		}
	}

	// Read messages for surface references
	messages, err := db.ReadMessages(projectPath)
	if err != nil {
		return nil, err
	}
	// Collect all references targets and surface_message links
	for _, msg := range messages {
		if msg.References != nil && *msg.References != "" {
			required[*msg.References] = struct{}{}
		}
		if msg.SurfaceMessage != nil && *msg.SurfaceMessage != "" {
			required[*msg.SurfaceMessage] = struct{}{}
		}
	}

	// Read thread_message events to preserve messages added to threads
	threadsPath := filepath.Join(frayDir, "threads.jsonl")
	if threadLines, err := readJSONLLines(threadsPath); err == nil {
		threadMsgs := make(map[string]struct{})
		for _, line := range threadLines {
			var envelope struct {
				Type        string `json:"type"`
				MessageGUID string `json:"message_guid"`
			}
			if err := json.Unmarshal([]byte(line), &envelope); err != nil {
				continue
			}
			switch envelope.Type {
			case "thread_message":
				threadMsgs[envelope.MessageGUID] = struct{}{}
			case "thread_message_remove":
				delete(threadMsgs, envelope.MessageGUID)
			}
		}
		for id := range threadMsgs {
			required[id] = struct{}{}
		}
	}

	return required, nil
}

// readJSONLLines reads all non-empty lines from a JSONL file.
func readJSONLLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

// writeMessagesWithEvents writes messages and their related events to the JSONL file.
func writeMessagesWithEvents(path string, messages []db.MessageJSONLRecord, keepIDs map[string]struct{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Read original JSONL lines to preserve events
	originalLines, err := readJSONLLines(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var builder strings.Builder

	// Write messages first
	for _, record := range messages {
		record.Type = "message"
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}

	// Write events for kept messages
	for _, line := range originalLines {
		var envelope struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "message":
			// Already written above
			continue
		case "message_update":
			// Check if the updated message is being kept
			if _, ok := keepIDs[envelope.ID]; ok {
				builder.WriteString(line)
				builder.WriteByte('\n')
			}
		case "message_pin", "message_unpin":
			// These use message_guid instead of id
			var pinEvent struct {
				MessageGUID string `json:"message_guid"`
			}
			if err := json.Unmarshal([]byte(line), &pinEvent); err != nil {
				continue
			}
			if _, ok := keepIDs[pinEvent.MessageGUID]; ok {
				builder.WriteString(line)
				builder.WriteByte('\n')
			}
		case "message_move":
			var moveEvent struct {
				MessageGUID string `json:"message_guid"`
			}
			if err := json.Unmarshal([]byte(line), &moveEvent); err != nil {
				continue
			}
			if _, ok := keepIDs[moveEvent.MessageGUID]; ok {
				builder.WriteString(line)
				builder.WriteByte('\n')
			}
		}
	}

	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func resolveFrayDir(projectPath string) string {
	if strings.HasSuffix(projectPath, ".db") {
		return filepath.Dir(projectPath)
	}
	if filepath.Base(projectPath) == ".fray" {
		return projectPath
	}
	return filepath.Join(projectPath, ".fray")
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
	status, err := runGitCommand(root, "status", "--porcelain", ".fray/")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("uncommitted changes in .fray/. Commit first")
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
