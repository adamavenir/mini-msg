package command

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/adamavenir/fray/internal/db"
)

type pruneResult struct {
	Kept           int
	Archived       int
	HistoryPath    string
	ClearedHistory bool
}

func pruneMessages(projectPath string, keep int, pruneAll bool, home string, opts pruneProtectionOpts, beforeGUID string) (pruneResult, error) {
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
	requiredIDs, excludeIDs, err := collectRequiredMessageIDs(projectPath, opts)
	if err != nil {
		return pruneResult{}, err
	}

	kept := messages
	if beforeGUID != "" {
		// Find the message and keep it and everything after
		foundIdx := -1
		for i, msg := range messages {
			if msg.ID == beforeGUID {
				foundIdx = i
				break
			}
		}
		if foundIdx >= 0 {
			kept = messages[foundIdx:]
		}
		// If not found in this home, keep all (message might be in different thread)
	} else if pruneAll || keep == 0 {
		kept = nil
	} else if len(messages) > keep {
		kept = messages[len(messages)-keep:]
	}

	if len(kept) > 0 && len(kept) < len(messages) {
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

		// Add excluded IDs (--without filter: keep messages that lack required attributes)
		for id := range excludeIDs {
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
	// (thread-scoped pruning only affects the target home)
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

	// Write messages with their associated events
	if err := writeMessagesWithEvents(messagesPath, allKept, allKeptIDSet); err != nil {
		return pruneResult{}, err
	}

	archived := 0
	if !pruneAll {
		archived = len(messages)
	}

	return pruneResult{Kept: len(kept), Archived: archived, HistoryPath: historyPath, ClearedHistory: pruneAll}, nil
}

// pruneMessagesWithReaction prunes messages that have a specific reaction.
// This inverts the normal reaction protection - instead of protecting, it selects for pruning.
func pruneMessagesWithReaction(projectPath, home, reaction string) (pruneResult, error) {
	frayDir := resolveFrayDir(projectPath)
	messagesPath := filepath.Join(frayDir, "messages.jsonl")
	historyPath := filepath.Join(frayDir, "history.jsonl")

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
			// Message in different home - keep it
			otherMessages = append(otherMessages, msg)
		} else if _, hasReaction := messagesWithReaction[msg.ID]; hasReaction {
			// Message has target reaction - prune it
			prunedMessages = append(prunedMessages, msg)
		} else {
			// Message doesn't have target reaction - keep it
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

	// Build set of kept message IDs for event filtering
	keptIDSet := make(map[string]struct{}, len(allKept))
	for _, msg := range allKept {
		keptIDSet[msg.ID] = struct{}{}
	}

	// Write messages with their associated events
	if err := writeMessagesWithEvents(messagesPath, allKept, keptIDSet); err != nil {
		return pruneResult{}, err
	}

	return pruneResult{
		Kept:        len(keptMessages),
		Archived:    len(prunedMessages),
		HistoryPath: historyPath,
	}, nil
}
