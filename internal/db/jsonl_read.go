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

// ReadMessages reads message records and applies updates.
func ReadMessages(projectPath string) ([]MessageJSONLRecord, error) {
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
					existing.Reactions = normalizeReactions(reactions)
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

// GetMessageVersions returns the full version history for a single message.
func GetMessageVersions(projectPath string, messageID string) (*types.MessageVersionHistory, error) {
	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, messagesFile))
	if err != nil {
		return nil, err
	}

	type versionUpdate struct {
		body      string
		timestamp *int64
		reason    string
		seq       int
	}

	var original *MessageJSONLRecord
	var updates []versionUpdate
	var archivedAt *int64
	seq := 0

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
			if err := json.Unmarshal([]byte(line), &update); err != nil {
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
					updates = append(updates, versionUpdate{body: body, timestamp: editedAt, reason: reason, seq: seq})
					seq++
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

	frayDir := resolveFrayDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(frayDir, messagesFile))
	if err != nil {
		return nil, err
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

// ReadThreads reads thread records and subscription/membership events.
func ReadThreads(projectPath string) ([]ThreadJSONLRecord, []threadSubscriptionEvent, []threadMessageEvent, error) {
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

// ReadAgents reads agent JSONL records and applies updates.
func ReadAgents(projectPath string) ([]AgentJSONLRecord, error) {
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
			agentMap[update.AgentID] = existing
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

// ReadThreadMutes reads thread mute events from JSONL for rebuilding the database.
func ReadThreadMutes(projectPath string) ([]threadMuteEvent, error) {
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
