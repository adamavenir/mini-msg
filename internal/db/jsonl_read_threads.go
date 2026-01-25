package db

import (
	"encoding/json"
	"path/filepath"

	"github.com/adamavenir/fray/internal/types"
)

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
	deletedAt := make(map[string]int64)

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
			if _, ok := deletedAt[record.GUID]; ok {
				record = applyThreadDelete(record)
			}
			threadMap[record.GUID] = record
		case "thread_update":
			var update ThreadUpdateJSONLRecord
			if err := json.Unmarshal([]byte(line), &update); err != nil {
				continue
			}
			if _, ok := deletedAt[update.GUID]; ok {
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
		case "thread_delete":
			var record struct {
				ThreadID string `json:"thread_id"`
				GUID     string `json:"guid"`
				TS       int64  `json:"ts"`
			}
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				continue
			}
			threadID := record.ThreadID
			if threadID == "" {
				threadID = record.GUID
			}
			if threadID == "" {
				continue
			}
			deletedAt[threadID] = record.TS
			if existing, ok := threadMap[threadID]; ok {
				threadMap[threadID] = applyThreadDelete(existing)
			}
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
	deletedAt := make(map[string]int64)
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
			_, deleted := deletedAt[record.GUID]
			if deleted {
				record = applyThreadDelete(record)
			}
			threadMap[record.GUID] = record
			if !deleted {
				if updates, ok := pendingUpdates[record.GUID]; ok {
					for _, update := range updates {
						threadMap[record.GUID] = applyThreadUpdate(threadMap[record.GUID], update)
					}
					delete(pendingUpdates, record.GUID)
				}
			}
		case "thread_update":
			var update ThreadUpdateJSONLRecord
			if err := json.Unmarshal([]byte(event.Line), &update); err != nil {
				continue
			}
			if _, ok := deletedAt[update.GUID]; ok {
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
				TS       int64  `json:"ts"`
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
			deletedAt[threadID] = record.TS
			if existing, ok := threadMap[threadID]; ok {
				threadMap[threadID] = applyThreadDelete(existing)
			}
			delete(pendingUpdates, threadID)
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
	case "agent_descriptor":
		return parseTimestamp(raw, []string{"ts"})
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

func applyThreadDelete(existing ThreadJSONLRecord) ThreadJSONLRecord {
	existing.Status = string(types.ThreadStatusArchived)
	return existing
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

// AgentDescriptor represents shared metadata about an agent for onboarding.
type AgentDescriptor struct {
	AgentID      string
	DisplayName  *string
	Capabilities []string
	TS           int64
}
