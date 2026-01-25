package command

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/adamavenir/fray/internal/db"
)

// collectRequiredMessageIDs gathers message IDs that must be preserved for data integrity.
// Returns:
//   - required: IDs that must be kept (pins, anchors, question refs, surface refs, thread messages)
//   - excluded: IDs that should NOT be pruned due to --without filter (messages lacking required attributes)
func collectRequiredMessageIDs(projectPath string, opts pruneProtectionOpts) (map[string]struct{}, map[string]struct{}, error) {
	required := make(map[string]struct{})
	excluded := make(map[string]struct{})
	frayDir := resolveFrayDir(projectPath)

	// Read threads for anchor messages (always protected)
	threads, _, _, err := db.ReadThreads(projectPath)
	if err != nil {
		return nil, nil, err
	}
	for _, thread := range threads {
		if thread.AnchorMessageGUID != nil && *thread.AnchorMessageGUID != "" {
			required[*thread.AnchorMessageGUID] = struct{}{}
		}
	}

	// Read fave events and track currently faved messages
	faveEvents, err := db.ReadFaves(projectPath)
	if err != nil {
		return nil, nil, err
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
	// Only add to required if ProtectFaves is true
	if opts.ProtectFaves {
		for id := range favedMessages {
			required[id] = struct{}{}
		}
	}

	// Read reactions - track which messages have reactions
	reactions, err := db.ReadReactions(projectPath)
	if err != nil {
		return nil, nil, err
	}
	messagesWithReactions := make(map[string]struct{})
	for _, r := range reactions {
		messagesWithReactions[r.MessageGUID] = struct{}{}
	}
	// Only add to required if ProtectReacts is true
	if opts.ProtectReacts {
		for id := range messagesWithReactions {
			required[id] = struct{}{}
		}
	}

	// Read message pins (always protected - cannot be pruned)
	pinEvents, err := db.ReadMessagePins(projectPath)
	if err != nil {
		return nil, nil, err
	}
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

	// Read questions for message references (always protected)
	questions, err := db.ReadQuestions(projectPath)
	if err != nil {
		return nil, nil, err
	}
	for _, q := range questions {
		if q.AskedIn != nil && *q.AskedIn != "" {
			required[*q.AskedIn] = struct{}{}
		}
		if q.AnsweredIn != nil && *q.AnsweredIn != "" {
			required[*q.AnsweredIn] = struct{}{}
		}
	}

	// Read messages for surface references and replies
	messages, err := db.ReadMessages(projectPath)
	if err != nil {
		return nil, nil, err
	}

	// Build set of messages that have replies (for ProtectReplies)
	messagesWithReplies := make(map[string]struct{})
	for _, msg := range messages {
		if msg.ReplyTo != nil && *msg.ReplyTo != "" {
			messagesWithReplies[*msg.ReplyTo] = struct{}{}
		}
	}

	// Collect all references targets and surface_message links (always protected)
	for _, msg := range messages {
		if msg.References != nil && *msg.References != "" {
			required[*msg.References] = struct{}{}
		}
		if msg.SurfaceMessage != nil && *msg.SurfaceMessage != "" {
			required[*msg.SurfaceMessage] = struct{}{}
		}
	}

	// Only add replies to required if ProtectReplies is true
	if opts.ProtectReplies {
		for id := range messagesWithReplies {
			required[id] = struct{}{}
		}
	}

	// Read thread_message events to preserve messages added to threads (always protected)
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

	// Handle --without filtering: keep messages that LACK the required attributes
	// If --without faves is set, keep messages that don't have faves
	// If --without reacts is set, keep messages that don't have reactions
	// If --without replies is set, keep messages that don't have replies
	if opts.RequireFaves || opts.RequireReacts || opts.RequireReplies {
		for _, msg := range messages {
			hasRequiredAttribute := true

			if opts.RequireFaves {
				if _, hasFave := favedMessages[msg.ID]; !hasFave {
					hasRequiredAttribute = false
				}
			}
			if opts.RequireReacts && hasRequiredAttribute {
				if _, hasReact := messagesWithReactions[msg.ID]; !hasReact {
					hasRequiredAttribute = false
				}
			}
			if opts.RequireReplies && hasRequiredAttribute {
				if _, hasReply := messagesWithReplies[msg.ID]; !hasReply {
					hasRequiredAttribute = false
				}
			}

			// If message lacks any required attribute, exclude it from pruning
			if !hasRequiredAttribute {
				excluded[msg.ID] = struct{}{}
			}
		}
	}

	return required, excluded, nil
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
