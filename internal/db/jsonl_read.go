package db

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/types"
)

func readJSONLLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func readJSONLFile[T any](filePath string) ([]T, error) {
	lines, err := readJSONLLines(filePath)
	if err != nil {
		return nil, err
	}

	records := make([]T, 0, len(lines))
	for _, line := range lines {
		var record T
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		records = append(records, record)
	}

	return records, nil
}

type jsonlLine struct {
	Line    string
	Machine string
	Index   int
}

type orderedJSONLEvent struct {
	Line    string
	Machine string
	Seq     int64
	TS      int64
	Index   int
}

func readSharedJSONLLines(projectPath, fileName string) ([]jsonlLine, error) {
	dirs := GetSharedMachinesDirs(projectPath)
	if len(dirs) == 0 {
		return nil, nil
	}
	lines := make([]jsonlLine, 0)
	for _, dir := range dirs {
		machine := filepath.Base(dir)
		filePath := filepath.Join(dir, fileName)
		fileLines, err := readJSONLLines(filePath)
		if err != nil {
			return nil, err
		}
		for idx, line := range fileLines {
			lines = append(lines, jsonlLine{
				Line:    line,
				Machine: machine,
				Index:   idx,
			})
		}
	}
	return lines, nil
}

func readLocalRuntimeLines(projectPath string) ([]string, error) {
	return readJSONLLines(GetLocalRuntimePath(projectPath))
}

func parseRawEnvelope(line string) (map[string]json.RawMessage, string) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, ""
	}
	var typ string
	if rawType, ok := raw["type"]; ok {
		_ = json.Unmarshal(rawType, &typ)
	}
	return raw, typ
}

func parseSeq(raw map[string]json.RawMessage, fallback int64) int64 {
	if raw == nil {
		return fallback
	}
	if rawSeq, ok := raw["seq"]; ok {
		var seq int64
		if err := json.Unmarshal(rawSeq, &seq); err == nil {
			return seq
		}
	}
	return fallback
}

func parseTimestamp(raw map[string]json.RawMessage, fields []string) int64 {
	for _, field := range fields {
		value, ok := raw[field]
		if !ok {
			continue
		}
		if string(value) == "null" {
			continue
		}
		var ts int64
		if err := json.Unmarshal(value, &ts); err == nil {
			return ts
		}
	}
	return 0
}

func sortOrderedEvents(events []orderedJSONLEvent) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].TS != events[j].TS {
			return events[i].TS < events[j].TS
		}
		if events[i].Machine != events[j].Machine {
			return events[i].Machine < events[j].Machine
		}
		if events[i].Seq != events[j].Seq {
			return events[i].Seq < events[j].Seq
		}
		return events[i].Index < events[j].Index
	})
}

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
			existing, ok := messageMap[move.MessageGUID]
			if !ok {
				continue
			}
			existing.Home = move.NewHome
			messageMap[move.MessageGUID] = existing
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
	deleted := make(map[string]struct{})
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
			if _, ok := seen[record.ID]; !ok {
				seen[record.ID] = struct{}{}
				order = append(order, record.ID)
			}
			if _, ok := deleted[record.ID]; ok {
				continue
			}
			messageMap[record.ID] = record
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
			if _, ok := deleted[update.ID]; ok {
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
			if _, ok := deleted[move.MessageGUID]; ok {
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
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			if record.ID == "" {
				continue
			}
			deleted[record.ID] = struct{}{}
			delete(messageMap, record.ID)
		}
	}

	messages := make([]MessageJSONLRecord, 0, len(order))
	for _, id := range order {
		if _, ok := deleted[id]; ok {
			continue
		}
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
	lines, err := readSharedJSONLLines(projectPath, messagesFile)
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

// ReadQuestions reads question records and applies updates.
func ReadQuestions(projectPath string) ([]QuestionJSONLRecord, error) {
	if IsMultiMachineMode(projectPath) {
		return readQuestionsMerged(projectPath)
	}
	return readQuestionsLegacy(projectPath)
}

func readQuestionsLegacy(projectPath string) ([]QuestionJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, questionsFile))
	if err != nil {
		return nil, err
	}

	questionMap := make(map[string]QuestionJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "question":
			var record QuestionJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if record.Status == "" {
				record.Status = string(types.QuestionStatusUnasked)
			}
			if _, ok := seen[record.GUID]; !ok {
				seen[record.GUID] = struct{}{}
				order = append(order, record.GUID)
			}
			questionMap[record.GUID] = record
		case "question_update":
			var update QuestionUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			existing, ok := questionMap[update.GUID]
			if !ok {
				continue
			}
			if update.Status != nil {
				existing.Status = *update.Status
			}
			if update.ToAgent != nil {
				existing.ToAgent = update.ToAgent
			}
			if update.ThreadGUID != nil {
				existing.ThreadGUID = update.ThreadGUID
			}
			if update.AskedIn != nil {
				existing.AskedIn = update.AskedIn
			}
			if update.AnsweredIn != nil {
				existing.AnsweredIn = update.AnsweredIn
			}
			questionMap[update.GUID] = existing
		}
	}

	questions := make([]QuestionJSONLRecord, 0, len(order))
	for _, guid := range order {
		record, ok := questionMap[guid]
		if !ok {
			continue
		}
		questions = append(questions, record)
	}
	return questions, nil
}

func readQuestionsMerged(projectPath string) ([]QuestionJSONLRecord, error) {
	lines, err := readSharedJSONLLines(projectPath, questionsFile)
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
		ts := questionEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}
	sortOrderedEvents(events)

	questionMap := make(map[string]QuestionJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})
	pendingUpdates := make(map[string][]QuestionUpdateJSONLRecord)

	for _, event := range events {
		raw, typ := parseRawEnvelope(event.Line)
		if raw == nil || typ == "" {
			continue
		}
		switch typ {
		case "question":
			var record QuestionJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			if _, ok := seen[record.GUID]; !ok {
				seen[record.GUID] = struct{}{}
				order = append(order, record.GUID)
			}
			questionMap[record.GUID] = record
			if updates, ok := pendingUpdates[record.GUID]; ok {
				for _, update := range updates {
					questionMap[record.GUID] = applyQuestionUpdate(questionMap[record.GUID], update)
				}
				delete(pendingUpdates, record.GUID)
			}
		case "question_update":
			var update QuestionUpdateJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &update); err != nil {
				continue
			}
			if update.GUID == "" {
				continue
			}
			existing, ok := questionMap[update.GUID]
			if !ok {
				pendingUpdates[update.GUID] = append(pendingUpdates[update.GUID], update)
				continue
			}
			questionMap[update.GUID] = applyQuestionUpdate(existing, update)
		}
	}

	questions := make([]QuestionJSONLRecord, 0, len(order))
	for _, guid := range order {
		record, ok := questionMap[guid]
		if !ok {
			continue
		}
		questions = append(questions, record)
	}
	return questions, nil
}

func questionEventTimestamp(eventType string, raw map[string]json.RawMessage) int64 {
	switch eventType {
	case "question":
		return parseTimestamp(raw, []string{"created_at", "ts"})
	case "question_update":
		return parseTimestamp(raw, []string{"ts"})
	default:
		return 0
	}
}

func applyQuestionUpdate(existing QuestionJSONLRecord, update QuestionUpdateJSONLRecord) QuestionJSONLRecord {
	if update.Status != nil {
		existing.Status = *update.Status
	}
	if update.ToAgent != nil {
		existing.ToAgent = update.ToAgent
	}
	if update.ThreadGUID != nil {
		existing.ThreadGUID = update.ThreadGUID
	}
	if update.AskedIn != nil {
		existing.AskedIn = update.AskedIn
	}
	if update.AnsweredIn != nil {
		existing.AnsweredIn = update.AnsweredIn
	}
	return existing
}

// ReadThreads reads thread records and subscription/membership events.
func ReadThreads(projectPath string) ([]ThreadJSONLRecord, []threadSubscriptionEvent, []threadMessageEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readThreadsMerged(projectPath)
	}
	return readThreadsLegacy(projectPath)
}

func readThreadsLegacy(projectPath string) ([]ThreadJSONLRecord, []threadSubscriptionEvent, []threadMessageEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, threadsFile))
	if err != nil {
		return nil, nil, nil, err
	}

	threadMap := make(map[string]ThreadJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})

	subEvents := make([]threadSubscriptionEvent, 0)
	msgEvents := make([]threadMessageEvent, 0)

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "thread":
			var record ThreadJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if record.Status == "" {
				record.Status = string(types.ThreadStatusOpen)
			}
			if _, ok := seen[record.GUID]; !ok {
				seen[record.GUID] = struct{}{}
				order = append(order, record.GUID)
			}
			threadMap[record.GUID] = record
		case "thread_update":
			var update ThreadUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			existing, ok := threadMap[update.GUID]
			if !ok {
				continue
			}
			if update.Name != nil {
				existing.Name = *update.Name
			}
			if update.Status != nil {
				existing.Status = *update.Status
			}
			if update.ThreadType != nil {
				existing.ThreadType = *update.ThreadType
			}
			if update.ParentThread != nil {
				existing.ParentThread = update.ParentThread
			}
			if update.AnchorMessageGUID != nil {
				existing.AnchorMessageGUID = update.AnchorMessageGUID
			}
			if update.AnchorHidden != nil {
				existing.AnchorHidden = *update.AnchorHidden
			}
			if update.LastActivityAt != nil {
				existing.LastActivityAt = update.LastActivityAt
			}
			threadMap[update.GUID] = existing
		case "thread_subscribe":
			var event ThreadSubscribeJSONLRecord
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			subEvents = append(subEvents, threadSubscriptionEvent{
				Type:       event.Type,
				ThreadGUID: event.ThreadGUID,
				AgentID:    event.AgentID,
				At:         event.SubscribedAt,
			})
		case "thread_unsubscribe":
			var event ThreadUnsubscribeJSONLRecord
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			subEvents = append(subEvents, threadSubscriptionEvent{
				Type:       event.Type,
				ThreadGUID: event.ThreadGUID,
				AgentID:    event.AgentID,
				At:         event.UnsubscribedAt,
			})
		case "thread_message":
			var event ThreadMessageJSONLRecord
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			msgEvents = append(msgEvents, threadMessageEvent{
				Type:        event.Type,
				ThreadGUID:  event.ThreadGUID,
				MessageGUID: event.MessageGUID,
				AddedBy:     event.AddedBy,
				AddedAt:     event.AddedAt,
			})
		case "thread_message_remove":
			var event ThreadMessageRemoveJSONLRecord
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			msgEvents = append(msgEvents, threadMessageEvent{
				Type:        event.Type,
				ThreadGUID:  event.ThreadGUID,
				MessageGUID: event.MessageGUID,
				RemovedBy:   event.RemovedBy,
				RemovedAt:   event.RemovedAt,
			})
		}
	}

	threads := make([]ThreadJSONLRecord, 0, len(order))
	for _, guid := range order {
		record, ok := threadMap[guid]
		if !ok {
			continue
		}
		threads = append(threads, record)
	}

	return threads, subEvents, msgEvents, nil
}

func readThreadsMerged(projectPath string) ([]ThreadJSONLRecord, []threadSubscriptionEvent, []threadMessageEvent, error) {
	lines, err := readSharedJSONLLines(projectPath, threadsFile)
	if err != nil {
		return nil, nil, nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil || typ == "" {
			continue
		}
		seq := parseSeq(raw, int64(entry.Index))
		ts := threadEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}
	sortOrderedEvents(events)

	threadMap := make(map[string]ThreadJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})
	deleted := make(map[string]struct{})
	pendingUpdates := make(map[string][]ThreadUpdateJSONLRecord)

	subEvents := make([]threadSubscriptionEvent, 0)
	msgEvents := make([]threadMessageEvent, 0)

	for _, event := range events {
		raw, typ := parseRawEnvelope(event.Line)
		if raw == nil || typ == "" {
			continue
		}
		switch typ {
		case "thread":
			var record ThreadJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			if record.Status == "" {
				record.Status = string(types.ThreadStatusOpen)
			}
			if _, ok := seen[record.GUID]; !ok {
				seen[record.GUID] = struct{}{}
				order = append(order, record.GUID)
			}
			if _, ok := deleted[record.GUID]; ok {
				continue
			}
			threadMap[record.GUID] = record
			if updates, ok := pendingUpdates[record.GUID]; ok {
				for _, update := range updates {
					threadMap[record.GUID] = applyThreadUpdate(threadMap[record.GUID], update)
				}
				delete(pendingUpdates, record.GUID)
			}
		case "thread_update":
			var update ThreadUpdateJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &update); err != nil {
				continue
			}
			if _, ok := deleted[update.GUID]; ok {
				continue
			}
			existing, ok := threadMap[update.GUID]
			if !ok {
				pendingUpdates[update.GUID] = append(pendingUpdates[update.GUID], update)
				continue
			}
			threadMap[update.GUID] = applyThreadUpdate(existing, update)
		case "thread_delete":
			var record struct {
				ThreadID string `json:"thread_id"`
				GUID     string `json:"guid"`
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			threadID := record.ThreadID
			if threadID == "" {
				threadID = record.GUID
			}
			if threadID == "" {
				continue
			}
			deleted[threadID] = struct{}{}
			delete(threadMap, threadID)
		case "thread_subscribe":
			var eventRecord ThreadSubscribeJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &eventRecord); err != nil {
				continue
			}
			subEvents = append(subEvents, threadSubscriptionEvent{
				Type:       eventRecord.Type,
				ThreadGUID: eventRecord.ThreadGUID,
				AgentID:    eventRecord.AgentID,
				At:         eventRecord.SubscribedAt,
			})
		case "thread_unsubscribe":
			var eventRecord ThreadUnsubscribeJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &eventRecord); err != nil {
				continue
			}
			subEvents = append(subEvents, threadSubscriptionEvent{
				Type:       eventRecord.Type,
				ThreadGUID: eventRecord.ThreadGUID,
				AgentID:    eventRecord.AgentID,
				At:         eventRecord.UnsubscribedAt,
			})
		case "thread_message":
			var eventRecord ThreadMessageJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &eventRecord); err != nil {
				continue
			}
			msgEvents = append(msgEvents, threadMessageEvent{
				Type:        eventRecord.Type,
				ThreadGUID:  eventRecord.ThreadGUID,
				MessageGUID: eventRecord.MessageGUID,
				AddedBy:     eventRecord.AddedBy,
				AddedAt:     eventRecord.AddedAt,
			})
		case "thread_message_remove":
			var eventRecord ThreadMessageRemoveJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &eventRecord); err != nil {
				continue
			}
			msgEvents = append(msgEvents, threadMessageEvent{
				Type:        eventRecord.Type,
				ThreadGUID:  eventRecord.ThreadGUID,
				MessageGUID: eventRecord.MessageGUID,
				RemovedBy:   eventRecord.RemovedBy,
				RemovedAt:   eventRecord.RemovedAt,
			})
		}
	}

	threads := make([]ThreadJSONLRecord, 0, len(order))
	for _, guid := range order {
		if _, ok := deleted[guid]; ok {
			continue
		}
		record, ok := threadMap[guid]
		if !ok {
			continue
		}
		threads = append(threads, record)
	}

	return threads, subEvents, msgEvents, nil
}

func threadEventTimestamp(eventType string, raw map[string]json.RawMessage) int64 {
	switch eventType {
	case "thread":
		return parseTimestamp(raw, []string{"created_at", "ts"})
	case "thread_update":
		return parseTimestamp(raw, []string{"last_activity_at", "ts"})
	case "thread_subscribe":
		return parseTimestamp(raw, []string{"subscribed_at", "ts"})
	case "thread_unsubscribe":
		return parseTimestamp(raw, []string{"unsubscribed_at", "ts"})
	case "thread_message":
		return parseTimestamp(raw, []string{"added_at", "ts"})
	case "thread_message_remove":
		return parseTimestamp(raw, []string{"removed_at", "ts"})
	case "thread_pin":
		return parseTimestamp(raw, []string{"pinned_at", "ts"})
	case "thread_unpin":
		return parseTimestamp(raw, []string{"unpinned_at", "ts"})
	case "thread_mute":
		return parseTimestamp(raw, []string{"muted_at", "ts"})
	case "thread_unmute":
		return parseTimestamp(raw, []string{"unmuted_at", "ts"})
	case "thread_delete":
		return parseTimestamp(raw, []string{"ts"})
	default:
		return 0
	}
}

func agentStateEventTimestamp(eventType string, raw map[string]json.RawMessage) int64 {
	switch eventType {
	case "ghost_cursor":
		return parseTimestamp(raw, []string{"set_at", "ts"})
	case "cursor_clear":
		return parseTimestamp(raw, []string{"ts"})
	case "agent_fave":
		return parseTimestamp(raw, []string{"faved_at", "ts"})
	case "agent_unfave":
		return parseTimestamp(raw, []string{"unfaved_at", "ts"})
	case "fave_remove":
		return parseTimestamp(raw, []string{"ts"})
	case "role_hold":
		return parseTimestamp(raw, []string{"assigned_at", "ts"})
	case "role_drop":
		return parseTimestamp(raw, []string{"dropped_at", "ts"})
	case "role_play":
		return parseTimestamp(raw, []string{"started_at", "ts"})
	case "role_stop":
		return parseTimestamp(raw, []string{"stopped_at", "ts"})
	case "role_release":
		return parseTimestamp(raw, []string{"ts"})
	default:
		return 0
	}
}

func applyThreadUpdate(existing ThreadJSONLRecord, update ThreadUpdateJSONLRecord) ThreadJSONLRecord {
	if update.Name != nil {
		existing.Name = *update.Name
	}
	if update.Status != nil {
		existing.Status = *update.Status
	}
	if update.ThreadType != nil {
		existing.ThreadType = *update.ThreadType
	}
	if update.ParentThread != nil {
		existing.ParentThread = update.ParentThread
	}
	if update.AnchorMessageGUID != nil {
		existing.AnchorMessageGUID = update.AnchorMessageGUID
	}
	if update.AnchorHidden != nil {
		existing.AnchorHidden = *update.AnchorHidden
	}
	if update.LastActivityAt != nil {
		existing.LastActivityAt = update.LastActivityAt
	}
	return existing
}

// ReadAgents reads agent JSONL records and applies updates.
func ReadAgents(projectPath string) ([]AgentJSONLRecord, error) {
	if IsMultiMachineMode(projectPath) {
		return readAgentsRuntime(projectPath)
	}
	return readAgentsLegacy(projectPath)
}

func readAgentsLegacy(projectPath string) ([]AgentJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	agentMap := make(map[string]AgentJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "agent":
			var record AgentJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if _, ok := seen[record.AgentID]; !ok {
				seen[record.AgentID] = struct{}{}
				order = append(order, record.AgentID)
			}
			agentMap[record.AgentID] = record
		case "agent_update":
			var update AgentUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			existing, ok := agentMap[update.AgentID]
			if !ok {
				continue
			}
			if update.Status != nil {
				existing.Status = update.Status
			}
			if update.Purpose != nil {
				existing.Purpose = update.Purpose
			}
			if update.Avatar != nil {
				existing.Avatar = update.Avatar
			}
			if update.LastSeen != nil {
				existing.LastSeen = *update.LastSeen
			}
			if update.LeftAt != nil {
				existing.LeftAt = update.LeftAt
			}
			if update.Managed != nil {
				existing.Managed = *update.Managed
			}
			if update.Invoke != nil {
				existing.Invoke = update.Invoke
			}
			if update.Presence != nil {
				existing.Presence = *update.Presence
			}
			if update.MentionWatermark != nil {
				existing.MentionWatermark = update.MentionWatermark
			}
			if update.LastHeartbeat != nil {
				existing.LastHeartbeat = update.LastHeartbeat
			}
			if update.SessionMode != nil {
				existing.SessionMode = *update.SessionMode
			}
			if update.LastSessionID != nil {
				existing.LastSessionID = update.LastSessionID
			}
			agentMap[update.AgentID] = existing
		case "presence_event":
			var event PresenceEventJSONLRecord
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}
			existing, ok := agentMap[event.AgentID]
			if !ok {
				continue
			}
			existing.Presence = event.To
			agentMap[event.AgentID] = existing
			// session_start, session_end, session_heartbeat are events, not agent records
			// They are handled separately when needed
		}
	}

	agents := make([]AgentJSONLRecord, 0, len(order))
	for _, id := range order {
		record, ok := agentMap[id]
		if !ok {
			continue
		}
		agents = append(agents, record)
	}
	return agents, nil
}

func readAgentsRuntime(projectPath string) ([]AgentJSONLRecord, error) {
	lines, err := readLocalRuntimeLines(projectPath)
	if err != nil {
		return nil, err
	}

	agentMap := make(map[string]AgentJSONLRecord)
	order := make([]string, 0)
	seen := make(map[string]struct{})

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "agent":
			var record AgentJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if _, ok := seen[record.AgentID]; !ok {
				seen[record.AgentID] = struct{}{}
				order = append(order, record.AgentID)
			}
			agentMap[record.AgentID] = record
		case "agent_update":
			var update AgentUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			existing, ok := agentMap[update.AgentID]
			if !ok {
				continue
			}
			if update.Status != nil {
				existing.Status = update.Status
			}
			if update.Purpose != nil {
				existing.Purpose = update.Purpose
			}
			if update.Avatar != nil {
				existing.Avatar = update.Avatar
			}
			if update.LastSeen != nil {
				existing.LastSeen = *update.LastSeen
			}
			if update.LeftAt != nil {
				existing.LeftAt = update.LeftAt
			}
			if update.Managed != nil {
				existing.Managed = *update.Managed
			}
			if update.Invoke != nil {
				existing.Invoke = update.Invoke
			}
			if update.Presence != nil {
				existing.Presence = *update.Presence
			}
			if update.MentionWatermark != nil {
				existing.MentionWatermark = update.MentionWatermark
			}
			if update.LastHeartbeat != nil {
				existing.LastHeartbeat = update.LastHeartbeat
			}
			if update.SessionMode != nil {
				existing.SessionMode = *update.SessionMode
			}
			if update.LastSessionID != nil {
				existing.LastSessionID = update.LastSessionID
			}
			agentMap[update.AgentID] = existing
		}
	}

	agents := make([]AgentJSONLRecord, 0, len(order))
	for _, id := range order {
		record, ok := agentMap[id]
		if !ok {
			continue
		}
		agents = append(agents, record)
	}
	return agents, nil
}

type threadSubscriptionEvent struct {
	Type       string
	ThreadGUID string
	AgentID    string
	At         int64
}

type threadMessageEvent struct {
	Type        string
	ThreadGUID  string
	MessageGUID string
	AddedBy     string
	AddedAt     int64
	RemovedBy   string
	RemovedAt   int64
}

// threadPinEvent represents a pin or unpin event for threads.
type threadPinEvent struct {
	Type       string
	ThreadGUID string
	PinnedBy   string
	PinnedAt   int64
	UnpinnedAt int64
}

// threadMuteEvent represents a mute or unmute event for threads.
type threadMuteEvent struct {
	Type       string
	ThreadGUID string
	AgentID    string
	MutedAt    int64
	ExpiresAt  *int64
	UnmutedAt  int64
}

// ReadThreadPins reads thread pin events from JSONL for rebuilding the database.
func ReadThreadPins(projectPath string) ([]threadPinEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readThreadPinsMerged(projectPath)
	}
	return readThreadPinsLegacy(projectPath)
}

func readThreadPinsLegacy(projectPath string) ([]threadPinEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, threadsFile))
	if err != nil {
		return nil, err
	}

	var events []threadPinEvent

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "thread_pin":
			var pin ThreadPinJSONLRecord
			if err := json.Unmarshal([]byte(line), &pin); err != nil {
				continue
			}
			events = append(events, threadPinEvent{
				Type:       pin.Type,
				ThreadGUID: pin.ThreadGUID,
				PinnedBy:   pin.PinnedBy,
				PinnedAt:   pin.PinnedAt,
			})
		case "thread_unpin":
			var unpin ThreadUnpinJSONLRecord
			if err := json.Unmarshal([]byte(line), &unpin); err != nil {
				continue
			}
			events = append(events, threadPinEvent{
				Type:       unpin.Type,
				ThreadGUID: unpin.ThreadGUID,
				UnpinnedAt: unpin.UnpinnedAt,
			})
		}
	}

	return events, nil
}

func readThreadPinsMerged(projectPath string) ([]threadPinEvent, error) {
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
		if typ != "thread_pin" && typ != "thread_unpin" {
			continue
		}
		seq := parseSeq(raw, int64(entry.Index))
		ts := threadEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}
	sortOrderedEvents(events)

	var pins []threadPinEvent
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "thread_pin":
			var record ThreadPinJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			pins = append(pins, threadPinEvent{
				Type:       record.Type,
				ThreadGUID: record.ThreadGUID,
				PinnedBy:   record.PinnedBy,
				PinnedAt:   record.PinnedAt,
			})
		case "thread_unpin":
			var record ThreadUnpinJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			pins = append(pins, threadPinEvent{
				Type:       record.Type,
				ThreadGUID: record.ThreadGUID,
				UnpinnedAt: record.UnpinnedAt,
			})
		}
	}
	return pins, nil
}

// ReadThreadMutes reads thread mute events from JSONL for rebuilding the database.
func ReadThreadMutes(projectPath string) ([]threadMuteEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readThreadMutesMerged(projectPath)
	}
	return readThreadMutesLegacy(projectPath)
}

func readThreadMutesLegacy(projectPath string) ([]threadMuteEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, threadsFile))
	if err != nil {
		return nil, err
	}

	var events []threadMuteEvent

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "thread_mute":
			var mute ThreadMuteJSONLRecord
			if err := json.Unmarshal([]byte(line), &mute); err != nil {
				continue
			}
			events = append(events, threadMuteEvent{
				Type:       mute.Type,
				ThreadGUID: mute.ThreadGUID,
				AgentID:    mute.AgentID,
				MutedAt:    mute.MutedAt,
				ExpiresAt:  mute.ExpiresAt,
			})
		case "thread_unmute":
			var unmute ThreadUnmuteJSONLRecord
			if err := json.Unmarshal([]byte(line), &unmute); err != nil {
				continue
			}
			events = append(events, threadMuteEvent{
				Type:       unmute.Type,
				ThreadGUID: unmute.ThreadGUID,
				AgentID:    unmute.AgentID,
				UnmutedAt:  unmute.UnmutedAt,
			})
		}
	}

	return events, nil
}

func readThreadMutesMerged(projectPath string) ([]threadMuteEvent, error) {
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
		if typ != "thread_mute" && typ != "thread_unmute" {
			continue
		}
		seq := parseSeq(raw, int64(entry.Index))
		ts := threadEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}
	sortOrderedEvents(events)

	var mutes []threadMuteEvent
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "thread_mute":
			var record ThreadMuteJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			mutes = append(mutes, threadMuteEvent{
				Type:       record.Type,
				ThreadGUID: record.ThreadGUID,
				AgentID:    record.AgentID,
				MutedAt:    record.MutedAt,
				ExpiresAt:  record.ExpiresAt,
			})
		case "thread_unmute":
			var record ThreadUnmuteJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			mutes = append(mutes, threadMuteEvent{
				Type:       record.Type,
				ThreadGUID: record.ThreadGUID,
				AgentID:    record.AgentID,
				UnmutedAt:  record.UnmutedAt,
			})
		}
	}
	return mutes, nil
}

// ReadGhostCursors reads ghost cursor events from agents.jsonl for rebuilding the database.
// Ghost cursors track recommended read positions for session handoffs.
func ReadGhostCursors(projectPath string) ([]GhostCursorJSONLRecord, error) {
	if IsMultiMachineMode(projectPath) {
		return readGhostCursorsMerged(projectPath)
	}
	return readGhostCursorsLegacy(projectPath)
}

func readGhostCursorsLegacy(projectPath string) ([]GhostCursorJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	// Track latest cursor per (agent, home) pair
	type cursorKey struct {
		agentID string
		home    string
	}
	cursorMap := make(map[cursorKey]GhostCursorJSONLRecord)

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		if envelope.Type == "ghost_cursor" {
			var record GhostCursorJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			key := cursorKey{agentID: record.AgentID, home: record.Home}
			cursorMap[key] = record
		}
	}

	cursors := make([]GhostCursorJSONLRecord, 0, len(cursorMap))
	for _, cursor := range cursorMap {
		cursors = append(cursors, cursor)
	}
	return cursors, nil
}

func readGhostCursorsMerged(projectPath string) ([]GhostCursorJSONLRecord, error) {
	lines, err := readSharedJSONLLines(projectPath, agentStateFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil {
			continue
		}
		if typ != "ghost_cursor" && typ != "cursor_clear" {
			continue
		}
		seq := parseSeq(raw, int64(entry.Index))
		ts := agentStateEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}
	sortOrderedEvents(events)

	type cursorKey struct {
		agentID string
		home    string
	}
	cursorMap := make(map[cursorKey]GhostCursorJSONLRecord)

	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "ghost_cursor":
			var record GhostCursorJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			key := cursorKey{agentID: record.AgentID, home: record.Home}
			cursorMap[key] = record
		case "cursor_clear":
			var record struct {
				AgentID string `json:"agent_id"`
				Home    string `json:"home"`
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			if record.AgentID == "" || record.Home == "" {
				continue
			}
			key := cursorKey{agentID: record.AgentID, home: record.Home}
			delete(cursorMap, key)
		}
	}

	cursors := make([]GhostCursorJSONLRecord, 0, len(cursorMap))
	for _, cursor := range cursorMap {
		cursors = append(cursors, cursor)
	}
	return cursors, nil
}

// ReadReactions reads all reaction records from messages.jsonl.
func ReadReactions(projectPath string) ([]ReactionJSONLRecord, error) {
	if IsMultiMachineMode(projectPath) {
		return readReactionsMerged(projectPath)
	}
	return readReactionsLegacy(projectPath)
}

func readReactionsLegacy(projectPath string) ([]ReactionJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, messagesFile))
	if err != nil {
		return nil, err
	}

	var reactions []ReactionJSONLRecord
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		if envelope.Type == "reaction" {
			var record ReactionJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			reactions = append(reactions, record)
		}
	}
	return reactions, nil
}

func readReactionsMerged(projectPath string) ([]ReactionJSONLRecord, error) {
	lines, err := readSharedJSONLLines(projectPath, messagesFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil || typ != "reaction" {
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

	reactions := make([]ReactionJSONLRecord, 0, len(events))
	for _, event := range events {
		var record ReactionJSONLRecord
		if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
			continue
		}
		reactions = append(reactions, record)
	}
	return reactions, nil
}

// FaveEvent represents a fave or unfave event for rebuilding.
type FaveEvent struct {
	Type      string
	AgentID   string
	ItemType  string
	ItemGUID  string
	FavedAt   int64
	UnfavedAt int64
}

// ReadFaves reads fave events from agents.jsonl for rebuilding the database.
func ReadFaves(projectPath string) ([]FaveEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readFavesMerged(projectPath)
	}
	return readFavesLegacy(projectPath)
}

func readFavesLegacy(projectPath string) ([]FaveEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	var events []FaveEvent
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "agent_fave":
			var record AgentFaveJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, FaveEvent{
				Type:     record.Type,
				AgentID:  record.AgentID,
				ItemType: record.ItemType,
				ItemGUID: record.ItemGUID,
				FavedAt:  record.FavedAt,
			})
		case "agent_unfave":
			var record AgentUnfaveJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, FaveEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				ItemType:  record.ItemType,
				ItemGUID:  record.ItemGUID,
				UnfavedAt: record.UnfavedAt,
			})
		}
	}
	return events, nil
}

func readFavesMerged(projectPath string) ([]FaveEvent, error) {
	lines, err := readSharedJSONLLines(projectPath, agentStateFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil {
			continue
		}
		if typ != "agent_fave" && typ != "agent_unfave" && typ != "fave_remove" {
			continue
		}
		seq := parseSeq(raw, int64(entry.Index))
		ts := agentStateEventTimestamp(typ, raw)
		events = append(events, orderedJSONLEvent{
			Line:    entry.Line,
			Machine: entry.Machine,
			Seq:     seq,
			TS:      ts,
			Index:   entry.Index,
		})
	}
	sortOrderedEvents(events)

	result := make([]FaveEvent, 0, len(events))
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "agent_fave":
			var record AgentFaveJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, FaveEvent{
				Type:     record.Type,
				AgentID:  record.AgentID,
				ItemType: record.ItemType,
				ItemGUID: record.ItemGUID,
				FavedAt:  record.FavedAt,
			})
		case "agent_unfave":
			var record AgentUnfaveJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, FaveEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				ItemType:  record.ItemType,
				ItemGUID:  record.ItemGUID,
				UnfavedAt: record.UnfavedAt,
			})
		case "fave_remove":
			var record struct {
				AgentID  string `json:"agent_id"`
				ItemType string `json:"item_type"`
				ItemGUID string `json:"item_guid"`
				TS       int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, FaveEvent{
				Type:      "fave_remove",
				AgentID:   record.AgentID,
				ItemType:  record.ItemType,
				ItemGUID:  record.ItemGUID,
				UnfavedAt: record.TS,
			})
		}
	}
	return result, nil
}

type roleEvent struct {
	Type       string
	AgentID    string
	RoleName   string
	SessionID  *string
	AssignedAt int64
	DroppedAt  int64
	StartedAt  int64
	StoppedAt  int64
}

// ReadRoles reads role events from agents.jsonl for rebuilding the database.
func ReadRoles(projectPath string) ([]roleEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readRolesMerged(projectPath)
	}
	return readRolesLegacy(projectPath)
}

func readRolesLegacy(projectPath string) ([]roleEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	var events []roleEvent
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "role_hold":
			var record RoleHoldJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:       record.Type,
				AgentID:    record.AgentID,
				RoleName:   record.RoleName,
				AssignedAt: record.AssignedAt,
			})
		case "role_drop":
			var record RoleDropJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				DroppedAt: record.DroppedAt,
			})
		case "role_play":
			var record RolePlayJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				SessionID: record.SessionID,
				StartedAt: record.StartedAt,
			})
		case "role_stop":
			var record RoleStopJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				StoppedAt: record.StoppedAt,
			})
		}
	}
	return events, nil
}

func readRolesMerged(projectPath string) ([]roleEvent, error) {
	lines, err := readSharedJSONLLines(projectPath, agentStateFile)
	if err != nil {
		return nil, err
	}

	events := make([]orderedJSONLEvent, 0, len(lines))
	for _, entry := range lines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil {
			continue
		}
		switch typ {
		case "role_hold", "role_drop", "role_play", "role_stop", "role_release":
			seq := parseSeq(raw, int64(entry.Index))
			ts := agentStateEventTimestamp(typ, raw)
			events = append(events, orderedJSONLEvent{
				Line:    entry.Line,
				Machine: entry.Machine,
				Seq:     seq,
				TS:      ts,
				Index:   entry.Index,
			})
		}
	}
	sortOrderedEvents(events)

	result := make([]roleEvent, 0, len(events))
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(event.Line), &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case "role_hold":
			var record RoleHoldJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:       record.Type,
				AgentID:    record.AgentID,
				RoleName:   record.RoleName,
				AssignedAt: record.AssignedAt,
			})
		case "role_drop":
			var record RoleDropJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				DroppedAt: record.DroppedAt,
			})
		case "role_play":
			var record RolePlayJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				SessionID: record.SessionID,
				StartedAt: record.StartedAt,
			})
		case "role_stop":
			var record RoleStopJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:      record.Type,
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				StoppedAt: record.StoppedAt,
			})
		case "role_release":
			var record struct {
				AgentID  string `json:"agent_id"`
				RoleName string `json:"role_name"`
				TS       int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(event.Line), &record); err != nil {
				continue
			}
			result = append(result, roleEvent{
				Type:      "role_release",
				AgentID:   record.AgentID,
				RoleName:  record.RoleName,
				DroppedAt: record.TS,
			})
		}
	}
	return result, nil
}

// ReadPermissions reads permission requests from JSONL and applies updates.
func ReadPermissions(projectPath string) ([]types.PermissionRequest, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, permissionsFile))
	if err != nil {
		return nil, err
	}

	permMap := make(map[string]types.PermissionRequest)
	order := make([]string, 0)

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "permission_request":
			var record PermissionJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			req := types.PermissionRequest{
				GUID:        record.GUID,
				FromAgent:   record.FromAgent,
				SessionID:   record.SessionID,
				Tool:        record.Tool,
				Action:      record.Action,
				Rationale:   record.Rationale,
				Options:     record.Options,
				Status:      types.PermissionStatus(record.Status),
				ChosenIndex: record.ChosenIndex,
				RespondedBy: record.RespondedBy,
				CreatedAt:   record.CreatedAt,
				RespondedAt: record.RespondedAt,
			}
			if _, exists := permMap[record.GUID]; !exists {
				order = append(order, record.GUID)
			}
			permMap[record.GUID] = req

		case "permission_update":
			var update PermissionUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			if req, exists := permMap[update.GUID]; exists {
				req.Status = types.PermissionStatus(update.Status)
				req.ChosenIndex = update.ChosenIndex
				req.RespondedBy = &update.RespondedBy
				req.RespondedAt = &update.RespondedAt
				permMap[update.GUID] = req
			}
		}
	}

	result := make([]types.PermissionRequest, 0, len(order))
	for _, guid := range order {
		result = append(result, permMap[guid])
	}
	return result, nil
}

// ReadPermissionByGUID reads a specific permission request by GUID.
func ReadPermissionByGUID(projectPath, guid string) (*types.PermissionRequest, error) {
	perms, err := ReadPermissions(projectPath)
	if err != nil {
		return nil, err
	}
	for _, p := range perms {
		if p.GUID == guid {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("permission request not found: %s", guid)
}

// TriggerEvent represents a session trigger with its context.
type TriggerEvent struct {
	AgentID     string  `json:"agent_id"`
	SessionID   string  `json:"session_id"`
	TriggeredBy *string `json:"triggered_by,omitempty"`
	ThreadGUID  *string `json:"thread_guid,omitempty"`
	StartedAt   int64   `json:"started_at"`
	EndedAt     *int64  `json:"ended_at,omitempty"`
	ExitCode    *int    `json:"exit_code,omitempty"`
	DurationMs  *int64  `json:"duration_ms,omitempty"`
}

// PresenceEvent represents a presence state change for audit.
type PresenceEvent struct {
	AgentID string  `json:"agent_id"`
	From    string  `json:"from"`
	To      string  `json:"to"`
	Status  *string `json:"status,omitempty"`
	Reason  string  `json:"reason"`
	Source  string  `json:"source"`
	TS      int64   `json:"ts"`
}

// ReadTriggerEvents reads session start/end events from agents.jsonl.
// Returns events sorted by timestamp descending (most recent first).
// Deduplicates by session ID - each session appears once with its latest trigger info.
func ReadTriggerEvents(projectPath string) ([]TriggerEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readTriggerEventsRuntime(projectPath)
	}
	return readTriggerEventsLegacy(projectPath)
}

func readTriggerEventsLegacy(projectPath string) ([]TriggerEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	// Map session_id -> TriggerEvent for merging start/end
	sessionMap := make(map[string]*TriggerEvent)
	sessionSeen := make(map[string]bool)
	var sessionOrder []string

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "session_start":
			var record SessionStartJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			event := &TriggerEvent{
				AgentID:     record.AgentID,
				SessionID:   record.SessionID,
				TriggeredBy: record.TriggeredBy,
				ThreadGUID:  record.ThreadGUID,
				StartedAt:   record.StartedAt,
			}
			sessionMap[record.SessionID] = event
			// Only add to order once (first occurrence)
			if !sessionSeen[record.SessionID] {
				sessionSeen[record.SessionID] = true
				sessionOrder = append(sessionOrder, record.SessionID)
			}

		case "session_end":
			var record SessionEndJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if event, ok := sessionMap[record.SessionID]; ok {
				event.EndedAt = &record.EndedAt
				event.ExitCode = &record.ExitCode
				event.DurationMs = &record.DurationMs
			}
		}
	}

	// Build result in reverse order (most recent first)
	events := make([]TriggerEvent, 0, len(sessionOrder))
	for i := len(sessionOrder) - 1; i >= 0; i-- {
		if event, ok := sessionMap[sessionOrder[i]]; ok {
			events = append(events, *event)
		}
	}
	return events, nil
}

func readTriggerEventsRuntime(projectPath string) ([]TriggerEvent, error) {
	lines, err := readLocalRuntimeLines(projectPath)
	if err != nil {
		return nil, err
	}

	sessionMap := make(map[string]*TriggerEvent)
	sessionSeen := make(map[string]bool)
	var sessionOrder []string

	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "session_start":
			var record SessionStartJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			event := &TriggerEvent{
				AgentID:     record.AgentID,
				SessionID:   record.SessionID,
				TriggeredBy: record.TriggeredBy,
				ThreadGUID:  record.ThreadGUID,
				StartedAt:   record.StartedAt,
			}
			sessionMap[record.SessionID] = event
			if !sessionSeen[record.SessionID] {
				sessionSeen[record.SessionID] = true
				sessionOrder = append(sessionOrder, record.SessionID)
			}
		case "session_end":
			var record SessionEndJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			if event, ok := sessionMap[record.SessionID]; ok {
				event.EndedAt = &record.EndedAt
				event.ExitCode = &record.ExitCode
				event.DurationMs = &record.DurationMs
			}
		}
	}

	events := make([]TriggerEvent, 0, len(sessionOrder))
	for i := len(sessionOrder) - 1; i >= 0; i-- {
		if event, ok := sessionMap[sessionOrder[i]]; ok {
			events = append(events, *event)
		}
	}
	return events, nil
}

// ReadPresenceEvents reads all presence_event records from agents.jsonl.
// Returns events sorted by timestamp descending (most recent first).
func ReadPresenceEvents(projectPath string) ([]PresenceEvent, error) {
	if IsMultiMachineMode(projectPath) {
		return readPresenceEventsRuntime(projectPath)
	}
	return readPresenceEventsLegacy(projectPath)
}

func readPresenceEventsLegacy(projectPath string) ([]PresenceEvent, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, agentsFile))
	if err != nil {
		return nil, err
	}

	var events []PresenceEvent
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		if envelope.Type == "presence_event" {
			var record PresenceEventJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, PresenceEvent{
				AgentID: record.AgentID,
				From:    record.From,
				To:      record.To,
				Status:  record.Status,
				Reason:  record.Reason,
				Source:  record.Source,
				TS:      record.TS,
			})
		}
	}

	// Reverse to get most recent first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func readPresenceEventsRuntime(projectPath string) ([]PresenceEvent, error) {
	lines, err := readLocalRuntimeLines(projectPath)
	if err != nil {
		return nil, err
	}

	var events []PresenceEvent
	for _, line := range lines {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		if envelope.Type == "presence_event" {
			var record PresenceEventJSONLRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			events = append(events, PresenceEvent{
				AgentID: record.AgentID,
				From:    record.From,
				To:      record.To,
				Status:  record.Status,
				Reason:  record.Reason,
				Source:  record.Source,
				TS:      record.TS,
			})
		}
	}

	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}
