package db

import (
	"encoding/json"
	"path/filepath"

	"github.com/adamavenir/fray/internal/types"
)

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
