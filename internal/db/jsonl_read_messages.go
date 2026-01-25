package db

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/adamavenir/fray/internal/types"
)

// ReadMessages reads message records and applies updates.
func ReadMessages(projectPath string) ([]MessageJSONLRecord, error) {
	if IsMultiMachineMode(projectPath) {
		return readMessagesMerged(projectPath)
	}
	return readMessagesLegacy(projectPath)
}

func readMessagesLegacy(projectPath string) ([]MessageJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, messagesFile))
	if err != nil {
		return nil, err
	}

	messageMap := make(map[string]MessageJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})
	deletedAt := make(map[string]int64)

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "message":
			var record MessageJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if record.Home == "" {
				record.Home = "room"
			}
			if _, ok := seen[record.ID]; !ok {
				seen[record.ID] = struct{}{}
				order = append(order, record.ID)
			}
			if deletedTS, ok := deletedAt[record.ID]; ok {
				record = applyMessageDelete(record, deletedTS)
			}
			messageMap[record.ID] = record
		case "message_update":
			var update struct {
				ID         string          `json:"id"`
				Body       json.RawMessage `json:"body"`
				EditedAt   json.RawMessage `json:"edited_at"`
				ArchivedAt json.RawMessage `json:"archived_at"`
				Reactions  json.RawMessage `json:"reactions"`
			}
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			if _, ok := deletedAt[update.ID]; ok {
				continue
			}
			existing, ok := messageMap[update.ID]
			if !ok {
				continue
			}
			if update.Body != nil && string(update.Body) != "null" {
				var body string
				if err := json.Unmarshal(update.Body, &body); err == nil {
					existing.Body = body
				}
			}
			if update.EditedAt != nil {
				if string(update.EditedAt) == "null" {
					existing.EditedAt = nil
				} else {
					var editedAt int64
					if err := json.Unmarshal(update.EditedAt, &editedAt); err == nil {
						existing.EditedAt = &editedAt
					}
				}
			}
			if update.ArchivedAt != nil {
				if string(update.ArchivedAt) == "null" {
					existing.ArchivedAt = nil
				} else {
					var archivedAt int64
					if err := json.Unmarshal(update.ArchivedAt, &archivedAt); err == nil {
						existing.ArchivedAt = &archivedAt
					}
				}
			}
			if update.Reactions != nil && string(update.Reactions) != "null" {
				var reactions map[string][]string
				if err := json.Unmarshal(update.Reactions, &reactions); err == nil {
					existing.Reactions = normalizeReactionsLegacy(reactions)
				}
			}
			messageMap[update.ID] = existing
		case "message_move":
			var move MessageMoveJSONLRecord
			if err := json.Unmarshal([]byte(line), &move); err != nil {
				continue
			}
			if _, ok := deletedAt[move.MessageGUID]; ok {
				continue
			}
			existing, ok := messageMap[move.MessageGUID]
			if !ok {
				continue
			}
			existing.Home = move.NewHome
			messageMap[move.MessageGUID] = existing
		case "message_delete":
			var record struct {
				ID string `json:"id"`
				TS int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if record.ID == "" {
				continue
			}
			deletedAt[record.ID] = record.TS
			if existing, ok := messageMap[record.ID]; ok {
				messageMap[record.ID] = applyMessageDelete(existing, record.TS)
			}
		}
	}

	messages := make([]MessageJSONLRecord, 0, len(order))
	for _, id := range order {
		record, ok := messageMap[id]
		if !ok {
			continue
		}
		messages = append(messages, record)
	}
	return messages, nil
}

func readMessagesMerged(projectPath string) ([]MessageJSONLRecord, error) {
	lines, err := readSharedJSONLLines(projectPath, messagesFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil || typ == "" {
			continue
		}
		seq := parseSeq(raw, int64(entry.Index))
		ts := messageEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}

	sortOrderedEvents(events)

	messageMap := make(map[string]MessageJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})
	deletedAt := make(map[string]int64)
	pendingUpdates := make(map[string][]string)
	pendingMoves := make(map[string][]MessageMoveJSONLRecord)

	for _, event := range events {
		raw, typ := parseRawEnvelope(event.Line)
		if raw == nil || typ == "" {
			continue
		}
		switch typ {
		case "message":
			var record MessageJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			if record.Home == "" {
				record.Home = "room"
			}
			if record.Origin == "" {
				record.Origin = event.Machine
			}
			if _, ok := seen[record.ID]; !ok {
				seen[record.ID] = struct{}{}
				order = append(order, record.ID)
			}
			deletedTS, deleted := deletedAt[record.ID]
			if deleted {
				record = applyMessageDelete(record, deletedTS)
			}
			messageMap[record.ID] = record
			if !deleted {
				if updates, ok := pendingUpdates[record.ID]; ok {
					for _, updateLine := range updates {
						messageMap[record.ID] = applyMessageUpdate(messageMap[record.ID], []byte(updateLine))
					}
					delete(pendingUpdates, record.ID)
				}
				if moves, ok := pendingMoves[record.ID]; ok {
					for _, move := range moves {
						msg := messageMap[record.ID]
						msg.Home = move.NewHome
						messageMap[record.ID] = msg
					}
					delete(pendingMoves, record.ID)
				}
			}
		case "message_update":
			var update struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(event.Line), &update); err != nil {
				continue
			}
			if update.ID == "" {
				continue
			}
			if _, ok := deletedAt[update.ID]; ok {
				continue
			}
			existing, ok := messageMap[update.ID]
			if !ok {
				pendingUpdates[update.ID] = append(pendingUpdates[update.ID], event.Line)
				continue
			}
			messageMap[update.ID] = applyMessageUpdate(existing, []byte(event.Line))
		case "message_move":
			var move MessageMoveJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &move); err != nil {
				continue
			}
			if _, ok := deletedAt[move.MessageGUID]; ok {
				continue
			}
			existing, ok := messageMap[move.MessageGUID]
			if !ok {
				pendingMoves[move.MessageGUID] = append(pendingMoves[move.MessageGUID], move)
				continue
			}
			existing.Home = move.NewHome
			messageMap[move.MessageGUID] = existing
		case "message_delete":
			var record struct {
				ID string `json:"id"`
				TS int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			if record.ID == "" {
				continue
			}
			deletedAt[record.ID] = record.TS
			if existing, ok := messageMap[record.ID]; ok {
				messageMap[record.ID] = applyMessageDelete(existing, record.TS)
			}
			delete(pendingUpdates, record.ID)
			delete(pendingMoves, record.ID)
		}
	}

	messages := make([]MessageJSONLRecord, 0, len(order))
	for _, id := range order {
		record, ok := messageMap[id]
		if !ok {
			continue
		}
		messages = append(messages, record)
	}
	return messages, nil
}

func messageEventTimestamp(eventType string, raw map[string]json.RawMessage) int64 {
	switch eventType {
	case "message":
		return parseTimestamp(raw, []string{"ts"})
	case "message_update":
		return parseTimestamp(raw, []string{"edited_at", "archived_at", "ts"})
	case "message_move":
		return parseTimestamp(raw, []string{"moved_at", "ts"})
	case "message_pin":
		return parseTimestamp(raw, []string{"pinned_at", "ts"})
	case "message_unpin":
		return parseTimestamp(raw, []string{"unpinned_at", "ts"})
	case "reaction":
		return parseTimestamp(raw, []string{"reacted_at", "ts"})
	case "message_delete":
		return parseTimestamp(raw, []string{"ts"})
	default:
		return 0
	}
}

func applyMessageUpdate(existing MessageJSONLRecord, updateLine []byte) MessageJSONLRecord {
	var update struct {
		ID         string          `json:"id"`
		Body       json.RawMessage `json:"body"`
		EditedAt   json.RawMessage `json:"edited_at"`
		ArchivedAt json.RawMessage `json:"archived_at"`
		Reactions  json.RawMessage `json:"reactions"`
	}
	if err := json.Unmarshal(updateLine, &update); err != nil {
		return existing
	}
	if update.Body != nil && string(update.Body) != "null" {
		var body string
		if err := json.Unmarshal(update.Body, &body); err == nil {
			existing.Body = body
		}
	}
	if update.EditedAt != nil {
		if string(update.EditedAt) == "null" {
			existing.EditedAt = nil
		} else {
			var editedAt int64
			if err := json.Unmarshal(update.EditedAt, &editedAt); err == nil {
				existing.EditedAt = &editedAt
			}
		}
	}
	if update.ArchivedAt != nil {
		if string(update.ArchivedAt) == "null" {
			existing.ArchivedAt = nil
		} else {
			var archivedAt int64
			if err := json.Unmarshal(update.ArchivedAt, &archivedAt); err == nil {
				existing.ArchivedAt = &archivedAt
			}
		}
	}
	if update.Reactions != nil && string(update.Reactions) != "null" {
		var reactions map[string][]string
		if err := json.Unmarshal(update.Reactions, &reactions); err == nil {
			existing.Reactions = normalizeReactionsLegacy(reactions)
		}
	}
	return existing
}

func applyMessageDelete(existing MessageJSONLRecord, deletedAt int64) MessageJSONLRecord {
	existing.Body = "[deleted]"
	existing.ArchivedAt = &deletedAt
	return existing
}

// MessagePinEvent represents a pin or unpin event for rebuild.
type MessagePinEvent struct {
	Type        string
	MessageGUID string
	ThreadGUID  string
	PinnedBy    string
	PinnedAt    int64
	UnpinnedAt  int64
}

// ReadMessagePins reads message pin events from JSONL for rebuilding the database.
func ReadMessagePins(projectPath string) ([]MessagePinEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readMessagePinsMerged(projectPath)
	}
	return readMessagePinsLegacy(projectPath)
}

func readMessagePinsLegacy(projectPath string) ([]MessagePinEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, messagesFile))
	if err != nil {
		return nil, err
	}

	var events []MessagePinEvent

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "message_pin":
			var pin MessagePinJSONLRecord
			if err := json.Unmarshal([]byte(line), &pin); err != nil {
				continue
			}
			events = append(events, MessagePinEvent{
				Type:        pin.Type,
				MessageGUID: pin.MessageGUID,
				ThreadGUID:  pin.ThreadGUID,
				PinnedBy:    pin.PinnedBy,
				PinnedAt:    pin.PinnedAt,
			})
		case "message_unpin":
			var unpin MessageUnpinJSONLRecord
			if err := json.Unmarshal([]byte(line), &unpin); err != nil {
				continue
			}
			events = append(events, MessagePinEvent{
				Type:        unpin.Type,
				MessageGUID: unpin.MessageGUID,
				ThreadGUID:  unpin.ThreadGUID,
				UnpinnedAt:  unpin.UnpinnedAt,
			})
		}
	}

	return events, nil
}

func readMessagePinsMerged(projectPath string) ([]MessagePinEvent, error) {
	lines, err := readSharedJSONLLines(projectPath, threadsFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil {
			continue
		}
		if typ != "message_pin" && typ != "message_unpin" {
			continue
		}
		seq := parseSeq(raw, int64(entry.Index))
		ts := messageEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}
	sortOrderedEvents(events)

	var pins []MessagePinEvent
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "message_pin":
			var record MessagePinJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			pins = append(pins, MessagePinEvent{
				Type:        record.Type,
				MessageGUID: record.MessageGUID,
				ThreadGUID:  record.ThreadGUID,
				PinnedBy:    record.PinnedBy,
				PinnedAt:    record.PinnedAt,
			})
		case "message_unpin":
			var record MessageUnpinJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			pins = append(pins, MessagePinEvent{
				Type:        record.Type,
				MessageGUID: record.MessageGUID,
				ThreadGUID:  record.ThreadGUID,
				UnpinnedAt:  record.UnpinnedAt,
			})
		}
	}
	return pins, nil
}

// GetMessageVersions returns the full version history for a single message.
func GetMessageVersions(projectPath string, messageID string) (*types.MessageVersionHistory, error) {
	type versionUpdate struct {
		body      string
		timestamp *int64
		reason    string
		seq       int64
	}

	var original *MessageJSONLRecord
	var updates []versionUpdate
	var archivedAt *int64
	events := make([]orderedJSONLEvent, 0)

	if IsMultiMachineMode(projectPath) {
		entries, err := readSharedJSONLLines(projectPath, messagesFile)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			raw, typ := parseRawEnvelope(entry.Line)
			if raw == nil {
				continue
			}
			if typ != "message" && typ != "message_update" {
				continue
			}
			seq := parseSeq(raw, int64(entry.Index))
			ts := messageEventTimestamp(typ, raw)
			events = append(events, orderedJSONLEvent{
				Line:    entry.Line,
				Machine: entry.Machine,
				Seq:     seq,
				TS:      ts,
				Index:   entry.Index,
			})
		}
	} else {
		frayDir := resolveFrayDir(projectPath)
		lines, err := readJSONLLines(filepath.Join(frayDir, messagesFile))
		if err != nil {
			return nil, err
		}
		for idx, line := range lines {
			raw, typ := parseRawEnvelope(line)
			if raw == nil {
				continue
			}
			if typ != "message" && typ != "message_update" {
				continue
			}
			seq := parseSeq(raw, int64(idx))
			ts := messageEventTimestamp(typ, raw)
			events = append(events, orderedJSONLEvent{
				Line:    line,
				Machine: "",
				Seq:     seq,
				TS:      ts,
				Index:   idx,
			})
		}
	}

	sortOrderedEvents(events)

	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "message":
			var record MessageJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			if record.ID != messageID {
				continue
			}
			if record.Home == "" {
				record.Home = "room"
			}
			original = &record
			archivedAt = record.ArchivedAt
		case "message_update":
			var update struct {
				ID         string          `json:"id"`
				Body       json.RawMessage `json:"body"`
				EditedAt   json.RawMessage `json:"edited_at"`
				ArchivedAt json.RawMessage `json:"archived_at"`
				Reason     json.RawMessage `json:"reason"`
			}
			if err := json.Unmarshal([]byte(event.Line), &update); err != nil {
				continue
			}
			if update.ID != messageID {
				continue
			}

			if update.ArchivedAt != nil {
				if string(update.ArchivedAt) == "null" {
					archivedAt = nil
				} else {
					var value int64
					if err := json.Unmarshal(update.ArchivedAt, &value); err == nil {
						archivedAt = &value
					}
				}
			}

			if update.Body != nil && string(update.Body) != "null" {
				var body string
				if err := json.Unmarshal(update.Body, &body); err == nil {
					var editedAt *int64
					if update.EditedAt != nil && string(update.EditedAt) != "null" {
						var value int64
						if err := json.Unmarshal(update.EditedAt, &value); err == nil {
							editedAt = &value
						}
					}
					var reason string
					if update.Reason != nil && string(update.Reason) != "null" {
						_ = json.Unmarshal(update.Reason, &reason)
					}
					updates = append(updates, versionUpdate{body: body, timestamp: editedAt, reason: reason, seq: event.Seq})
				}
			}
		}
	}

	if original == nil {
		return nil, fmt.Errorf("message not found: %s", messageID)
	}

	for i := range updates {
		if updates[i].timestamp == nil {
			value := original.TS
			updates[i].timestamp = &value
		}
	}

	sort.SliceStable(updates, func(i, j int) bool {
		if *updates[i].timestamp == *updates[j].timestamp {
			return updates[i].seq < updates[j].seq
		}
		return *updates[i].timestamp < *updates[j].timestamp
	})

	versions := make([]types.MessageVersion, 0, len(updates)+1)
	versions = append(versions, types.MessageVersion{
		Version:    1,
		Body:       original.Body,
		Timestamp:  original.TS,
		IsOriginal: true,
	})

	for i, update := range updates {
		versions = append(versions, types.MessageVersion{
			Version:   i + 2,
			Body:      update.body,
			Timestamp: *update.timestamp,
			Reason:    update.reason,
		})
	}

	if len(versions) > 0 {
		versions[len(versions)-1].IsCurrent = true
	}

	history := &types.MessageVersionHistory{
		MessageID:    messageID,
		VersionCount: len(versions),
		IsArchived:   archivedAt != nil,
		Versions:     versions,
	}
	return history, nil
}

// GetMessageEditCounts returns the number of edits per message ID.
func GetMessageEditCounts(projectPath string, messageIDs []string) (map[string]int, error) {
	counts := make(map[string]int)
	if len(messageIDs) == 0 {
		return counts, nil
	}

	idSet := make(map[string]struct{}, len(messageIDs))
	for _, id := range messageIDs {
		if id == "" {
			continue
		}
		idSet[id] = struct{}{}
	}
	if len(idSet) == 0 {
		return counts, nil
	}

	var lines []string
	if IsMultiMachineMode(projectPath) {
		entries, err := readSharedJSONLLines(projectPath, messagesFile)
		if err != nil {
			return nil, err
		}
		lines = make([]string, 0, len(entries))
		for _, entry := range entries {
			lines = append(lines, entry.Line)
		}
	} else {
		frayDir := resolveFrayDir(projectPath)
		legacyLines, err := readJSONLLines(filepath.Join(frayDir, messagesFile))
		if err != nil {
			return nil, err
		}
		lines = legacyLines
	}

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.Type != "message_update" {
			continue
		}
		var update struct {
			ID         string          `json:"id"`
			Body       json.RawMessage `json:"body"`
			EditedAt   json.RawMessage `json:"edited_at"`
			ArchivedAt json.RawMessage `json:"archived_at"`
		}
		if err := json.Unmarshal([]byte(line), &update); err != nil {
			continue
		}
		if _, ok := idSet[update.ID]; !ok {
			continue
		}

		hasEdit := update.EditedAt != nil && string(update.EditedAt) != "null"
		if !hasEdit && update.Body != nil && string(update.Body) != "null" {
			if update.ArchivedAt == nil || string(update.ArchivedAt) == "null" {
				hasEdit = true
			}
		}
		if hasEdit {
			counts[update.ID]++
		}
	}

	return counts, nil
}

// ApplyMessageEditCounts populates edit metadata on messages from JSONL.
func ApplyMessageEditCounts(projectPath string, messages []types.Message) ([]types.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		ids = append(ids, msg.ID)
	}

	counts, err := GetMessageEditCounts(projectPath, ids)
	if err != nil {
		return messages, err
	}

	for i := range messages {
		count := counts[messages[i].ID]
		if count == 0 && messages[i].EditedAt != nil {
			count = 1
		}
		if count > 0 {
			messages[i].Edited = true
			messages[i].EditCount = count
		}
	}

	return messages, nil
}
