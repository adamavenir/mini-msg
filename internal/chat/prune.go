package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
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

func pruneMessages(projectPath string, keep int, pruneAll bool, home string) (pruneResult, error) {
	if keep < 0 {
		return pruneResult{}, fmt.Errorf("invalid --keep value: %d", keep)
	}

	frayDir := resolveFrayDir(projectPath)
	messagesPath := filepath.Join(frayDir, messagesJSONL)
	historyPath := filepath.Join(frayDir, historyJSONL)

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
	allMessages, err := db.ReadMessages(projectPath)
	if err != nil {
		return pruneResult{}, err
	}

	// Filter messages by home (thread-scoped pruning)
	var messages []db.MessageJSONLRecord
	var otherMessages []db.MessageJSONLRecord
	for _, msg := range allMessages {
		msgHome := msg.Home
		if msgHome == "" {
			msgHome = "room"
		}
		if msgHome == home {
			messages = append(messages, msg)
		} else {
			otherMessages = append(otherMessages, msg)
		}
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

	// Identify pruned messages (messages in target home that are not being kept)
	keptIDSet := make(map[string]struct{}, len(kept))
	for _, msg := range kept {
		keptIDSet[msg.ID] = struct{}{}
	}

	var prunedMessages []db.MessageJSONLRecord
	for _, msg := range messages {
		if _, ok := keptIDSet[msg.ID]; !ok {
			prunedMessages = append(prunedMessages, msg)
		}
	}

	// Generate tombstone if messages were pruned
	var tombstone *db.MessageJSONLRecord
	if len(prunedMessages) > 0 {
		tombstone = createTombstone(prunedMessages, home)
	}

	// Combine kept messages from target home with all messages from other homes
	allKept := make([]db.MessageJSONLRecord, 0, len(kept)+len(otherMessages))
	allKept = append(allKept, otherMessages...)
	allKept = append(allKept, kept...)

	// Add tombstone to kept messages if one was created
	if tombstone != nil {
		allKept = append(allKept, *tombstone)
		keptIDSet[tombstone.ID] = struct{}{}
	}

	// Build full set of kept message IDs for event filtering
	allKeptIDSet := make(map[string]struct{}, len(allKept))
	for _, msg := range allKept {
		allKeptIDSet[msg.ID] = struct{}{}
	}

	// Write messages
	if err := writeMessages(messagesPath, allKept); err != nil {
		return pruneResult{}, err
	}

	archived := 0
	if !pruneAll {
		archived = len(messages)
	}

	return pruneResult{Kept: len(kept), Archived: archived, HistoryPath: historyPath, ClearedHistory: pruneAll}, nil
}

// pruneMessagesWithReaction prunes messages that have a specific reaction.
func pruneMessagesWithReaction(projectPath, home, reaction string) (pruneResult, error) {
	frayDir := resolveFrayDir(projectPath)
	messagesPath := filepath.Join(frayDir, messagesJSONL)
	historyPath := filepath.Join(frayDir, historyJSONL)

	// Read all messages
	allMessages, err := db.ReadMessages(projectPath)
	if err != nil {
		return pruneResult{}, err
	}

	// Read reactions to find messages with the target reaction
	reactions, err := db.ReadReactions(projectPath)
	if err != nil {
		return pruneResult{}, err
	}

	// Build set of message IDs that have the target reaction
	messagesWithReaction := make(map[string]struct{})
	for _, r := range reactions {
		if r.Emoji == reaction {
			messagesWithReaction[r.MessageGUID] = struct{}{}
		}
	}

	// Separate messages by home and reaction status
	var keptMessages []db.MessageJSONLRecord
	var prunedMessages []db.MessageJSONLRecord
	var otherMessages []db.MessageJSONLRecord

	for _, msg := range allMessages {
		msgHome := msg.Home
		if msgHome == "" {
			msgHome = "room"
		}

		if msgHome != home {
			otherMessages = append(otherMessages, msg)
		} else if _, hasReaction := messagesWithReaction[msg.ID]; hasReaction {
			prunedMessages = append(prunedMessages, msg)
		} else {
			keptMessages = append(keptMessages, msg)
		}
	}

	// Archive pruned messages to history.jsonl
	if len(prunedMessages) > 0 {
		if data, err := os.ReadFile(messagesPath); err == nil {
			if strings.TrimSpace(string(data)) != "" {
				if err := appendFile(historyPath, data); err != nil {
					return pruneResult{}, err
				}
			}
		} else if !os.IsNotExist(err) {
			return pruneResult{}, err
		}
	}

	// Generate tombstone if messages were pruned
	var tombstone *db.MessageJSONLRecord
	if len(prunedMessages) > 0 {
		tombstone = createTombstone(prunedMessages, home)
	}

	// Combine kept messages
	allKept := make([]db.MessageJSONLRecord, 0, len(keptMessages)+len(otherMessages)+1)
	allKept = append(allKept, otherMessages...)
	allKept = append(allKept, keptMessages...)

	if tombstone != nil {
		allKept = append(allKept, *tombstone)
	}

	// Write messages
	if err := writeMessages(messagesPath, allKept); err != nil {
		return pruneResult{}, err
	}

	return pruneResult{
		Kept:        len(keptMessages),
		Archived:    len(prunedMessages),
		HistoryPath: historyPath,
	}, nil
}

// createTombstone generates a tombstone message for pruned messages.
func createTombstone(prunedMessages []db.MessageJSONLRecord, home string) *db.MessageJSONLRecord {
	if len(prunedMessages) == 0 {
		return nil
	}

	// Collect unique participants
	participants := make(map[string]struct{})
	for _, msg := range prunedMessages {
		if msg.FromAgent != "" && msg.FromAgent != "system" {
			participants[msg.FromAgent] = struct{}{}
		}
	}
	var participantList []string
	for p := range participants {
		participantList = append(participantList, "@"+p)
	}

	// Find first and last message IDs
	firstID := prunedMessages[0].ID
	lastID := prunedMessages[len(prunedMessages)-1].ID

	body := fmt.Sprintf("pruned: %d messages between %s from #%s to #%s",
		len(prunedMessages),
		strings.Join(participantList, ", "),
		firstID,
		lastID,
	)

	now := time.Now().Unix()
	msgID, err := core.GenerateGUID("msg")
	if err != nil {
		msgID = fmt.Sprintf("msg-%d", now)
	}
	return &db.MessageJSONLRecord{
		Type:      "message",
		ID:        msgID,
		Home:      home,
		FromAgent: "system",
		Body:      body,
		MsgType:   types.MessageTypeTombstone,
		TS:        now,
	}
}

// collectRequiredMessageIDs gathers message IDs that must be preserved for data integrity.
func collectRequiredMessageIDs(projectPath string) (map[string]struct{}, error) {
	required := make(map[string]struct{})

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

	// Read fave events and track currently faved messages
	faveEvents, err := db.ReadFaves(projectPath)
	if err != nil {
		return nil, err
	}
	favedMessages := make(map[string]struct{})
	for _, event := range faveEvents {
		if event.ItemType != "message" {
			continue
		}
		if event.Type == "agent_fave" {
			favedMessages[event.ItemGUID] = struct{}{}
		} else if event.Type == "agent_unfave" {
			delete(favedMessages, event.ItemGUID)
		}
	}
	for id := range favedMessages {
		required[id] = struct{}{}
	}

	// Read reactions - any message with reactions is protected
	reactions, err := db.ReadReactions(projectPath)
	if err != nil {
		return nil, err
	}
	for _, r := range reactions {
		required[r.MessageGUID] = struct{}{}
	}

	return required, nil
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

func projectRootFromPath(projectPath string) string {
	frayDir := resolveFrayDir(projectPath)
	return filepath.Dir(frayDir)
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
