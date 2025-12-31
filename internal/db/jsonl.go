package db

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

const (
	messagesFile      = "messages.jsonl"
	agentsFile        = "agents.jsonl"
	questionsFile     = "questions.jsonl"
	threadsFile       = "threads.jsonl"
	projectConfigFile = "fray-config.json"
)

// MessageJSONLRecord represents a message entry in JSONL.
type MessageJSONLRecord struct {
	Type           string              `json:"type"`
	ID             string              `json:"id"`
	ChannelID      *string             `json:"channel_id"`
	Home           string              `json:"home,omitempty"`
	FromAgent      string              `json:"from_agent"`
	Body           string              `json:"body"`
	Mentions       []string            `json:"mentions"`
	Reactions      map[string][]string `json:"reactions,omitempty"`
	MsgType        types.MessageType   `json:"message_type"`
	References     *string             `json:"references,omitempty"`
	SurfaceMessage *string             `json:"surface_message,omitempty"`
	ReplyTo        *string             `json:"reply_to"`
	TS             int64               `json:"ts"`
	EditedAt       *int64              `json:"edited_at"`
	ArchivedAt     *int64              `json:"archived_at"`
}

// MessageUpdateJSONLRecord represents a message update entry in JSONL.
type MessageUpdateJSONLRecord struct {
	Type       string               `json:"type"`
	ID         string               `json:"id"`
	Body       *string              `json:"body,omitempty"`
	EditedAt   *int64               `json:"edited_at,omitempty"`
	ArchivedAt *int64               `json:"archived_at,omitempty"`
	Reactions  *map[string][]string `json:"reactions,omitempty"`
	Reason     *string              `json:"reason,omitempty"`
}

// QuestionJSONLRecord represents a question entry in JSONL.
type QuestionJSONLRecord struct {
	Type       string  `json:"type"`
	GUID       string  `json:"guid"`
	Re         string  `json:"re"`
	FromAgent  string  `json:"from_agent"`
	ToAgent    *string `json:"to,omitempty"`
	Status     string  `json:"status"`
	ThreadGUID *string `json:"thread_guid,omitempty"`
	AskedIn    *string `json:"asked_in,omitempty"`
	AnsweredIn *string `json:"answered_in,omitempty"`
	CreatedAt  int64   `json:"created_at"`
}

// QuestionUpdateJSONLRecord represents a question update entry in JSONL.
type QuestionUpdateJSONLRecord struct {
	Type       string  `json:"type"`
	GUID       string  `json:"guid"`
	Status     *string `json:"status,omitempty"`
	ToAgent    *string `json:"to,omitempty"`
	ThreadGUID *string `json:"thread_guid,omitempty"`
	AskedIn    *string `json:"asked_in,omitempty"`
	AnsweredIn *string `json:"answered_in,omitempty"`
}

// ThreadJSONLRecord represents a thread entry in JSONL.
type ThreadJSONLRecord struct {
	Type         string   `json:"type"`
	GUID         string   `json:"guid"`
	Name         string   `json:"name"`
	ParentThread *string  `json:"parent_thread,omitempty"`
	Subscribed   []string `json:"subscribed,omitempty"`
	Status       string   `json:"status"`
	CreatedAt    int64    `json:"created_at"`
}

// ThreadUpdateJSONLRecord represents a thread update entry in JSONL.
type ThreadUpdateJSONLRecord struct {
	Type         string  `json:"type"`
	GUID         string  `json:"guid"`
	Name         *string `json:"name,omitempty"`
	Status       *string `json:"status,omitempty"`
	ParentThread *string `json:"parent_thread,omitempty"`
}

// ThreadSubscribeJSONLRecord represents a subscription event.
type ThreadSubscribeJSONLRecord struct {
	Type         string `json:"type"`
	ThreadGUID   string `json:"thread_guid"`
	AgentID      string `json:"agent_id"`
	SubscribedAt int64  `json:"subscribed_at"`
}

// ThreadUnsubscribeJSONLRecord represents an unsubscribe event.
type ThreadUnsubscribeJSONLRecord struct {
	Type           string `json:"type"`
	ThreadGUID     string `json:"thread_guid"`
	AgentID        string `json:"agent_id"`
	UnsubscribedAt int64  `json:"unsubscribed_at"`
}

// ThreadMessageJSONLRecord represents a thread membership event.
type ThreadMessageJSONLRecord struct {
	Type        string `json:"type"`
	ThreadGUID  string `json:"thread_guid"`
	MessageGUID string `json:"message_guid"`
	AddedBy     string `json:"added_by"`
	AddedAt     int64  `json:"added_at"`
}

// ThreadMessageRemoveJSONLRecord represents a removal event.
type ThreadMessageRemoveJSONLRecord struct {
	Type        string `json:"type"`
	ThreadGUID  string `json:"thread_guid"`
	MessageGUID string `json:"message_guid"`
	RemovedBy   string `json:"removed_by"`
	RemovedAt   int64  `json:"removed_at"`
}

// AgentJSONLRecord represents an agent entry in JSONL.
type AgentJSONLRecord struct {
	Type         string  `json:"type"`
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	GlobalName   *string `json:"global_name,omitempty"`
	HomeChannel  *string `json:"home_channel,omitempty"`
	CreatedAt    *string `json:"created_at,omitempty"`
	ActiveStatus *string `json:"active_status,omitempty"`
	AgentID      string  `json:"agent_id"`
	Status       *string `json:"status,omitempty"`
	Purpose      *string `json:"purpose,omitempty"`
	Goal         *string `json:"goal,omitempty"`
	Bio          *string `json:"bio,omitempty"`
	RegisteredAt int64   `json:"registered_at"`
	LastSeen     int64   `json:"last_seen"`
	LeftAt       *int64  `json:"left_at"`
}

// ProjectKnownAgent stores per-project known-agent data.
type ProjectKnownAgent struct {
	Name        *string  `json:"name,omitempty"`
	GlobalName  *string  `json:"global_name,omitempty"`
	HomeChannel *string  `json:"home_channel,omitempty"`
	CreatedAt   *string  `json:"created_at,omitempty"`
	FirstSeen   *string  `json:"first_seen,omitempty"`
	Status      *string  `json:"status,omitempty"`
	Nicks       []string `json:"nicks,omitempty"`
}

// ProjectConfig represents the per-project config file.
type ProjectConfig struct {
	Version     int                          `json:"version"`
	ChannelID   string                       `json:"channel_id,omitempty"`
	ChannelName string                       `json:"channel_name,omitempty"`
	CreatedAt   string                       `json:"created_at,omitempty"`
	KnownAgents map[string]ProjectKnownAgent `json:"known_agents,omitempty"`
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

func ensureDir(dirPath string) error {
	return os.MkdirAll(dirPath, 0o755)
}

func appendJSONLine(filePath string, record any) error {
	if err := ensureDir(filepath.Dir(filePath)); err != nil {
		return err
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}

	return nil
}

func touchDatabaseFile(projectPath string) {
	frayDir := resolveFrayDir(projectPath)
	if strings.HasSuffix(projectPath, ".db") {
		frayDir = filepath.Dir(projectPath)
	}
	path := filepath.Join(frayDir, "fray.db")
	_, err := os.Stat(path)
	if err != nil {
		return
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

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

// AppendMessage appends a message record to JSONL.
func AppendMessage(projectPath string, message types.Message) error {
	frayDir := resolveFrayDir(projectPath)
	home := message.Home
	if home == "" {
		home = "room"
	}
	record := MessageJSONLRecord{
		Type:           "message",
		ID:             message.ID,
		ChannelID:      message.ChannelID,
		Home:           home,
		FromAgent:      message.FromAgent,
		Body:           message.Body,
		Mentions:       message.Mentions,
		Reactions:      normalizeReactions(message.Reactions),
		MsgType:        message.Type,
		References:     message.References,
		SurfaceMessage: message.SurfaceMessage,
		ReplyTo:        message.ReplyTo,
		TS:             message.TS,
		EditedAt:       message.EditedAt,
		ArchivedAt:     message.ArchivedAt,
	}

	if err := appendJSONLine(filepath.Join(frayDir, messagesFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageUpdate appends an update record to JSONL.
func AppendMessageUpdate(projectPath string, update MessageUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "message_update"
	if err := appendJSONLine(filepath.Join(frayDir, messagesFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgent appends an agent record to JSONL.
func AppendAgent(projectPath string, agent types.Agent) error {
	frayDir := resolveFrayDir(projectPath)
	config, err := ReadProjectConfig(projectPath)
	if err != nil {
		return err
	}

	channelName := ""
	channelID := ""
	if config != nil {
		channelName = config.ChannelName
		channelID = config.ChannelID
	}

	name := agent.AgentID
	globalName := name
	if channelName != "" {
		globalName = fmt.Sprintf("%s-%s", channelName, name)
	}

	createdAt := time.Unix(agent.RegisteredAt, 0).UTC().Format(time.RFC3339)
	activeStatus := "active"
	if agent.LeftAt != nil {
		activeStatus = "inactive"
	}

	record := AgentJSONLRecord{
		Type:         "agent",
		ID:           agent.GUID,
		Name:         name,
		GlobalName:   &globalName,
		HomeChannel:  nil,
		CreatedAt:    &createdAt,
		ActiveStatus: &activeStatus,
		AgentID:      agent.AgentID,
		Status:       agent.Status,
		Purpose:      agent.Purpose,
		RegisteredAt: agent.RegisteredAt,
		LastSeen:     agent.LastSeen,
		LeftAt:       agent.LeftAt,
	}

	if channelID != "" {
		record.HomeChannel = &channelID
	}

	if err := appendJSONLine(filepath.Join(frayDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendQuestion appends a question record to JSONL.
func AppendQuestion(projectPath string, question types.Question) error {
	frayDir := resolveFrayDir(projectPath)
	record := QuestionJSONLRecord{
		Type:       "question",
		GUID:       question.GUID,
		Re:         question.Re,
		FromAgent:  question.FromAgent,
		ToAgent:    question.ToAgent,
		Status:     string(question.Status),
		ThreadGUID: question.ThreadGUID,
		AskedIn:    question.AskedIn,
		AnsweredIn: question.AnsweredIn,
		CreatedAt:  question.CreatedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, questionsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendQuestionUpdate appends a question update record to JSONL.
func AppendQuestionUpdate(projectPath string, update QuestionUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "question_update"
	if err := appendJSONLine(filepath.Join(frayDir, questionsFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThread appends a thread record to JSONL.
func AppendThread(projectPath string, thread types.Thread, subscribed []string) error {
	frayDir := resolveFrayDir(projectPath)
	record := ThreadJSONLRecord{
		Type:         "thread",
		GUID:         thread.GUID,
		Name:         thread.Name,
		ParentThread: thread.ParentThread,
		Subscribed:   subscribed,
		Status:       string(thread.Status),
		CreatedAt:    thread.CreatedAt,
	}
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUpdate appends a thread update record to JSONL.
func AppendThreadUpdate(projectPath string, update ThreadUpdateJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	update.Type = "thread_update"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadSubscribe appends a thread subscribe event to JSONL.
func AppendThreadSubscribe(projectPath string, event ThreadSubscribeJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_subscribe"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnsubscribe appends a thread unsubscribe event to JSONL.
func AppendThreadUnsubscribe(projectPath string, event ThreadUnsubscribeJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_unsubscribe"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMessage appends a thread message membership event to JSONL.
func AppendThreadMessage(projectPath string, event ThreadMessageJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_message"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMessageRemove appends a thread message removal event to JSONL.
func AppendThreadMessageRemove(projectPath string, event ThreadMessageRemoveJSONLRecord) error {
	frayDir := resolveFrayDir(projectPath)
	event.Type = "thread_message_remove"
	if err := appendJSONLine(filepath.Join(frayDir, threadsFile), event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// UpdateProjectConfig merges updates into the project config.
func UpdateProjectConfig(projectPath string, updates ProjectConfig) (*ProjectConfig, error) {
	frayDir := resolveFrayDir(projectPath)
	if err := ensureDir(frayDir); err != nil {
		return nil, err
	}

	existing, err := ReadProjectConfig(projectPath)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing = &ProjectConfig{
			Version:     1,
			KnownAgents: map[string]ProjectKnownAgent{},
		}
	}

	if existing.KnownAgents == nil {
		existing.KnownAgents = map[string]ProjectKnownAgent{}
	}

	for id, agent := range updates.KnownAgents {
		prior := existing.KnownAgents[id]
		existing.KnownAgents[id] = mergeKnownAgent(prior, agent)
	}

	if updates.Version != 0 {
		existing.Version = updates.Version
	}
	if updates.ChannelID != "" {
		existing.ChannelID = updates.ChannelID
	}
	if updates.ChannelName != "" {
		existing.ChannelName = updates.ChannelName
	}
	if updates.CreatedAt != "" {
		existing.CreatedAt = updates.CreatedAt
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	configPath := filepath.Join(frayDir, projectConfigFile)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return nil, err
	}

	return existing, nil
}

func mergeKnownAgent(existing, updates ProjectKnownAgent) ProjectKnownAgent {
	merged := existing
	if updates.Name != nil {
		merged.Name = updates.Name
	}
	if updates.GlobalName != nil {
		merged.GlobalName = updates.GlobalName
	}
	if updates.HomeChannel != nil {
		merged.HomeChannel = updates.HomeChannel
	}
	if updates.CreatedAt != nil {
		merged.CreatedAt = updates.CreatedAt
	}
	if updates.FirstSeen != nil {
		merged.FirstSeen = updates.FirstSeen
	}
	if updates.Status != nil {
		merged.Status = updates.Status
	}
	if updates.Nicks != nil {
		merged.Nicks = updates.Nicks
	}
	return merged
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

// ReadAgents reads agent JSONL records.
func ReadAgents(projectPath string) ([]AgentJSONLRecord, error) {
	frayDir := resolveFrayDir(projectPath)
	return readJSONLFile[AgentJSONLRecord](filepath.Join(frayDir, agentsFile))
}

// ReadProjectConfig reads the project config file.
func ReadProjectConfig(projectPath string) (*ProjectConfig, error) {
	frayDir := resolveFrayDir(projectPath)
	path := filepath.Join(frayDir, projectConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var config ProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// RebuildDatabaseFromJSONL resets the SQLite cache using JSONL sources.
func RebuildDatabaseFromJSONL(db DBTX, projectPath string) error {
	messages, err := ReadMessages(projectPath)
	if err != nil {
		return err
	}
	questions, err := ReadQuestions(projectPath)
	if err != nil {
		return err
	}
	threads, subEvents, msgEvents, err := ReadThreads(projectPath)
	if err != nil {
		return err
	}
	agents, err := ReadAgents(projectPath)
	if err != nil {
		return err
	}
	config, err := ReadProjectConfig(projectPath)
	if err != nil {
		return err
	}

	if _, err := db.Exec("DROP TABLE IF EXISTS fray_messages"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_agents"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_read_receipts"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_questions"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_thread_messages"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_thread_subscriptions"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS fray_threads"); err != nil {
		return err
	}
	if err := initSchemaWith(db); err != nil {
		return err
	}

	if config != nil && config.ChannelID != "" {
		if _, err := db.Exec("INSERT OR REPLACE INTO fray_config (key, value) VALUES (?, ?)", "channel_id", config.ChannelID); err != nil {
			return err
		}
		if config.ChannelName != "" {
			if _, err := db.Exec("INSERT OR REPLACE INTO fray_config (key, value) VALUES (?, ?)", "channel_name", config.ChannelName); err != nil {
				return err
			}
		}
	}

	insertAgent := `
		INSERT OR REPLACE INTO fray_agents (
			guid, agent_id, status, purpose, registered_at, last_seen, left_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	for _, agent := range agents {
		status := agent.Status
		if status == nil {
			status = agent.Goal
		}
		purpose := agent.Purpose
		if purpose == nil {
			purpose = agent.Bio
		}

		if _, err := db.Exec(insertAgent,
			agent.ID,
			agent.AgentID,
			status,
			purpose,
			agent.RegisteredAt,
			agent.LastSeen,
			agent.LeftAt,
		); err != nil {
			return err
		}
	}

	insertMessage := `
		INSERT OR REPLACE INTO fray_messages (
			guid, ts, channel_id, home, from_agent, body, mentions, type, "references", surface_message, reply_to, edited_at, archived_at, reactions
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, message := range messages {
		mentionsJSON, err := json.Marshal(message.Mentions)
		if err != nil {
			return err
		}
		reactionsJSON, err := json.Marshal(normalizeReactions(message.Reactions))
		if err != nil {
			return err
		}
		msgType := message.MsgType
		if msgType == "" {
			msgType = types.MessageTypeAgent
		}

		home := message.Home
		if home == "" {
			home = "room"
		}

		if _, err := db.Exec(insertMessage,
			message.ID,
			message.TS,
			message.ChannelID,
			home,
			message.FromAgent,
			message.Body,
			string(mentionsJSON),
			msgType,
			message.References,
			message.SurfaceMessage,
			message.ReplyTo,
			message.EditedAt,
			message.ArchivedAt,
			string(reactionsJSON),
		); err != nil {
			return err
		}
	}

	if len(questions) > 0 {
		insertQuestion := `
			INSERT OR REPLACE INTO fray_questions (
				guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		for _, question := range questions {
			status := question.Status
			if status == "" {
				status = string(types.QuestionStatusUnasked)
			}
			if _, err := db.Exec(insertQuestion,
				question.GUID,
				question.Re,
				question.FromAgent,
				question.ToAgent,
				status,
				question.ThreadGUID,
				question.AskedIn,
				question.AnsweredIn,
				question.CreatedAt,
			); err != nil {
				return err
			}
		}
	}

	if len(threads) > 0 {
		insertThread := `
			INSERT OR REPLACE INTO fray_threads (
				guid, name, parent_thread, status, created_at
			) VALUES (?, ?, ?, ?, ?)
		`
		for _, thread := range threads {
			status := thread.Status
			if status == "" {
				status = string(types.ThreadStatusOpen)
			}
			if _, err := db.Exec(insertThread,
				thread.GUID,
				thread.Name,
				thread.ParentThread,
				status,
				thread.CreatedAt,
			); err != nil {
				return err
			}
		}

		subscriptions := make(map[string]map[string]int64)
		for _, thread := range threads {
			if len(thread.Subscribed) == 0 {
				continue
			}
			set := make(map[string]int64, len(thread.Subscribed))
			for _, agentID := range thread.Subscribed {
				if agentID == "" {
					continue
				}
				set[agentID] = thread.CreatedAt
			}
			if len(set) > 0 {
				subscriptions[thread.GUID] = set
			}
		}

		for _, event := range subEvents {
			set, ok := subscriptions[event.ThreadGUID]
			if !ok {
				set = make(map[string]int64)
				subscriptions[event.ThreadGUID] = set
			}
			switch event.Type {
			case "thread_subscribe":
				set[event.AgentID] = event.At
			case "thread_unsubscribe":
				delete(set, event.AgentID)
			}
		}

		for threadGUID, set := range subscriptions {
			for agentID, subscribedAt := range set {
				if _, err := db.Exec(`
					INSERT OR REPLACE INTO fray_thread_subscriptions (thread_guid, agent_id, subscribed_at)
					VALUES (?, ?, ?)
				`, threadGUID, agentID, subscribedAt); err != nil {
					return err
				}
			}
		}

		threadMessages := make(map[string]map[string]ThreadMessageJSONLRecord)
		for _, event := range msgEvents {
			switch event.Type {
			case "thread_message":
				if _, ok := threadMessages[event.ThreadGUID]; !ok {
					threadMessages[event.ThreadGUID] = make(map[string]ThreadMessageJSONLRecord)
				}
				threadMessages[event.ThreadGUID][event.MessageGUID] = ThreadMessageJSONLRecord{
					ThreadGUID:  event.ThreadGUID,
					MessageGUID: event.MessageGUID,
					AddedBy:     event.AddedBy,
					AddedAt:     event.AddedAt,
				}
			case "thread_message_remove":
				if set, ok := threadMessages[event.ThreadGUID]; ok {
					delete(set, event.MessageGUID)
				}
			}
		}

		for _, messages := range threadMessages {
			for _, entry := range messages {
				if _, err := db.Exec(`
					INSERT OR REPLACE INTO fray_thread_messages (thread_guid, message_guid, added_by, added_at)
					VALUES (?, ?, ?, ?)
				`, entry.ThreadGUID, entry.MessageGUID, entry.AddedBy, entry.AddedAt); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
