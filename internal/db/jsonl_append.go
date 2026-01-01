package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

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
		Options:    question.Options,
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
