package db

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/types"
)

const (
	messagesFile      = "messages.jsonl"
	agentsFile        = "agents.jsonl"
	projectConfigFile = "mm-config.json"
)

// MessageJSONLRecord represents a message entry in JSONL.
type MessageJSONLRecord struct {
	Type       string            `json:"type"`
	ID         string            `json:"id"`
	ChannelID  *string           `json:"channel_id"`
	FromAgent  string            `json:"from_agent"`
	Body       string            `json:"body"`
	Mentions   []string          `json:"mentions"`
	MsgType    types.MessageType `json:"message_type"`
	ReplyTo    *string           `json:"reply_to"`
	TS         int64             `json:"ts"`
	EditedAt   *int64            `json:"edited_at"`
	ArchivedAt *int64            `json:"archived_at"`
}

// MessageUpdateJSONLRecord represents a message update entry in JSONL.
type MessageUpdateJSONLRecord struct {
	Type       string  `json:"type"`
	ID         string  `json:"id"`
	Body       *string `json:"body,omitempty"`
	EditedAt   *int64  `json:"edited_at,omitempty"`
	ArchivedAt *int64  `json:"archived_at,omitempty"`
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

func resolveMMDir(projectPath string) string {
	if strings.HasSuffix(projectPath, ".db") {
		return filepath.Dir(projectPath)
	}
	if filepath.Base(projectPath) == ".mm" {
		return projectPath
	}
	return filepath.Join(projectPath, ".mm")
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
	mmDir := resolveMMDir(projectPath)
	if strings.HasSuffix(projectPath, ".db") {
		mmDir = filepath.Dir(projectPath)
	}
	path := filepath.Join(mmDir, "mm.db")
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
	mmDir := resolveMMDir(projectPath)
	record := MessageJSONLRecord{
		Type:       "message",
		ID:         message.ID,
		ChannelID:  message.ChannelID,
		FromAgent:  message.FromAgent,
		Body:       message.Body,
		Mentions:   message.Mentions,
		MsgType:    message.Type,
		ReplyTo:    message.ReplyTo,
		TS:         message.TS,
		EditedAt:   message.EditedAt,
		ArchivedAt: message.ArchivedAt,
	}

	if err := appendJSONLine(filepath.Join(mmDir, messagesFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageUpdate appends an update record to JSONL.
func AppendMessageUpdate(projectPath string, update MessageUpdateJSONLRecord) error {
	mmDir := resolveMMDir(projectPath)
	update.Type = "message_update"
	if err := appendJSONLine(filepath.Join(mmDir, messagesFile), update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgent appends an agent record to JSONL.
func AppendAgent(projectPath string, agent types.Agent) error {
	mmDir := resolveMMDir(projectPath)
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

	if err := appendJSONLine(filepath.Join(mmDir, agentsFile), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// UpdateProjectConfig merges updates into the project config.
func UpdateProjectConfig(projectPath string, updates ProjectConfig) (*ProjectConfig, error) {
	mmDir := resolveMMDir(projectPath)
	if err := ensureDir(mmDir); err != nil {
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

	configPath := filepath.Join(mmDir, projectConfigFile)
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
	mmDir := resolveMMDir(projectPath)
	lines, err := readJSONLLines(filepath.Join(mmDir, messagesFile))
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

// ReadAgents reads agent JSONL records.
func ReadAgents(projectPath string) ([]AgentJSONLRecord, error) {
	mmDir := resolveMMDir(projectPath)
	return readJSONLFile[AgentJSONLRecord](filepath.Join(mmDir, agentsFile))
}

// ReadProjectConfig reads the project config file.
func ReadProjectConfig(projectPath string) (*ProjectConfig, error) {
	mmDir := resolveMMDir(projectPath)
	path := filepath.Join(mmDir, projectConfigFile)
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
	agents, err := ReadAgents(projectPath)
	if err != nil {
		return err
	}
	config, err := ReadProjectConfig(projectPath)
	if err != nil {
		return err
	}

	if _, err := db.Exec("DROP TABLE IF EXISTS mm_messages"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS mm_agents"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS mm_read_receipts"); err != nil {
		return err
	}
	if err := initSchemaWith(db); err != nil {
		return err
	}

	if config != nil && config.ChannelID != "" {
		if _, err := db.Exec("INSERT OR REPLACE INTO mm_config (key, value) VALUES (?, ?)", "channel_id", config.ChannelID); err != nil {
			return err
		}
		if config.ChannelName != "" {
			if _, err := db.Exec("INSERT OR REPLACE INTO mm_config (key, value) VALUES (?, ?)", "channel_name", config.ChannelName); err != nil {
				return err
			}
		}
	}

	insertAgent := `
		INSERT OR REPLACE INTO mm_agents (
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
		INSERT OR REPLACE INTO mm_messages (
			guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, message := range messages {
		mentionsJSON, err := json.Marshal(message.Mentions)
		if err != nil {
			return err
		}
		msgType := message.MsgType
		if msgType == "" {
			msgType = types.MessageTypeAgent
		}

		if _, err := db.Exec(insertMessage,
			message.ID,
			message.TS,
			message.ChannelID,
			message.FromAgent,
			message.Body,
			string(mentionsJSON),
			msgType,
			message.ReplyTo,
			message.EditedAt,
			message.ArchivedAt,
		); err != nil {
			return err
		}
	}

	return nil
}
