package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

func TestAppendAndReadMessages(t *testing.T) {
	projectDir := t.TempDir()

	message := types.Message{
		ID:        "msg-abc12345",
		TS:        123,
		ChannelID: strPtr("ch-00000000"),
		FromAgent: "alice.1",
		Body:      "hello",
		Mentions:  []string{"bob.1"},
		Type:      types.MessageTypeAgent,
	}

	if err := AppendMessage(projectDir, message); err != nil {
		t.Fatalf("append message: %v", err)
	}

	readBack, err := ReadMessages(projectDir)
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	if len(readBack) != 1 {
		t.Fatalf("expected 1 message, got %d", len(readBack))
	}
	if readBack[0].ID != message.ID {
		t.Fatalf("expected id %s, got %s", message.ID, readBack[0].ID)
	}
	if len(readBack[0].Mentions) != 1 || readBack[0].Mentions[0] != "bob.1" {
		t.Fatalf("expected mentions to roundtrip")
	}
	if readBack[0].ChannelID == nil || *readBack[0].ChannelID != "ch-00000000" {
		t.Fatalf("expected channel id to roundtrip")
	}
}

func TestGetMessageVersions(t *testing.T) {
	projectDir := t.TempDir()

	message := types.Message{
		ID:        "msg-abc12345",
		TS:        1000,
		FromAgent: "alice",
		Body:      "original text",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	}

	if err := AppendMessage(projectDir, message); err != nil {
		t.Fatalf("append message: %v", err)
	}

	body1 := "first edit"
	editedAt1 := int64(2000)
	reason1 := "first reason"
	if err := AppendMessageUpdate(projectDir, MessageUpdateJSONLRecord{
		ID:       message.ID,
		Body:     &body1,
		EditedAt: &editedAt1,
		Reason:   &reason1,
	}); err != nil {
		t.Fatalf("append update 1: %v", err)
	}

	body2 := "second edit"
	editedAt2 := int64(3000)
	reason2 := "second reason"
	if err := AppendMessageUpdate(projectDir, MessageUpdateJSONLRecord{
		ID:       message.ID,
		Body:     &body2,
		EditedAt: &editedAt2,
		Reason:   &reason2,
	}); err != nil {
		t.Fatalf("append update 2: %v", err)
	}

	archivedAt := int64(4000)
	if err := AppendMessageUpdate(projectDir, MessageUpdateJSONLRecord{
		ID:         message.ID,
		ArchivedAt: &archivedAt,
	}); err != nil {
		t.Fatalf("append update archived: %v", err)
	}

	history, err := GetMessageVersions(projectDir, message.ID)
	if err != nil {
		t.Fatalf("get versions: %v", err)
	}
	if history.VersionCount != 3 {
		t.Fatalf("expected 3 versions, got %d", history.VersionCount)
	}
	if len(history.Versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(history.Versions))
	}
	if !history.IsArchived {
		t.Fatalf("expected archived history")
	}
	if history.Versions[0].Body != "original text" || !history.Versions[0].IsOriginal {
		t.Fatalf("expected original version")
	}
	if history.Versions[1].Body != "first edit" || history.Versions[1].Timestamp != editedAt1 {
		t.Fatalf("expected first edit")
	}
	if history.Versions[1].Reason != reason1 {
		t.Fatalf("expected first reason")
	}
	if history.Versions[2].Body != "second edit" || !history.Versions[2].IsCurrent {
		t.Fatalf("expected current version")
	}
	if history.Versions[2].Reason != reason2 {
		t.Fatalf("expected second reason")
	}
}

func TestApplyMessageEditCounts(t *testing.T) {
	projectDir := t.TempDir()

	msg1 := types.Message{
		ID:        "msg-aaa11111",
		TS:        100,
		FromAgent: "alice",
		Body:      "original",
		Type:      types.MessageTypeAgent,
	}
	msg2 := types.Message{
		ID:        "msg-bbb22222",
		TS:        200,
		FromAgent: "bob",
		Body:      "reacted",
		Type:      types.MessageTypeAgent,
	}

	if err := AppendMessage(projectDir, msg1); err != nil {
		t.Fatalf("append message 1: %v", err)
	}
	if err := AppendMessage(projectDir, msg2); err != nil {
		t.Fatalf("append message 2: %v", err)
	}

	body := "edited text"
	editedAt := int64(300)
	reason := "fix typo"
	if err := AppendMessageUpdate(projectDir, MessageUpdateJSONLRecord{
		ID:       msg1.ID,
		Body:     &body,
		EditedAt: &editedAt,
		Reason:   &reason,
	}); err != nil {
		t.Fatalf("append edit update: %v", err)
	}

	reactions := map[string][]string{":+1:": {"alice"}}
	if err := AppendMessageUpdate(projectDir, MessageUpdateJSONLRecord{
		ID:        msg2.ID,
		Reactions: &reactions,
	}); err != nil {
		t.Fatalf("append reaction update: %v", err)
	}

	annotated, err := ApplyMessageEditCounts(projectDir, []types.Message{msg1, msg2})
	if err != nil {
		t.Fatalf("apply edit counts: %v", err)
	}
	if !annotated[0].Edited || annotated[0].EditCount != 1 {
		t.Fatalf("expected msg1 edited count 1, got edited=%v count=%d", annotated[0].Edited, annotated[0].EditCount)
	}
	if annotated[1].Edited || annotated[1].EditCount != 0 {
		t.Fatalf("expected msg2 unedited, got edited=%v count=%d", annotated[1].Edited, annotated[1].EditCount)
	}
}

func TestAppendAndReadAgents(t *testing.T) {
	projectDir := t.TempDir()

	agent := types.Agent{
		GUID:         "usr-abc12345",
		AgentID:      "alice.1",
		Status:       strPtr("test"),
		Purpose:      nil,
		RegisteredAt: 100,
		LastSeen:     120,
		LeftAt:       nil,
	}

	if err := AppendAgent(projectDir, agent); err != nil {
		t.Fatalf("append agent: %v", err)
	}

	readBack, err := ReadAgents(projectDir)
	if err != nil {
		t.Fatalf("read agents: %v", err)
	}
	if len(readBack) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(readBack))
	}
	if readBack[0].ID != agent.GUID {
		t.Fatalf("expected id %s, got %s", agent.GUID, readBack[0].ID)
	}
	if readBack[0].AgentID != agent.AgentID {
		t.Fatalf("expected agent id %s, got %s", agent.AgentID, readBack[0].AgentID)
	}
}

func TestUpdateProjectConfigMergesKnownAgents(t *testing.T) {
	projectDir := t.TempDir()

	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{
		ChannelID:   "ch-11111111",
		ChannelName: "Alpha",
		KnownAgents: map[string]ProjectKnownAgent{
			"usr-aaa11111": {Name: strPtr("alice.1")},
		},
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	if _, err := UpdateProjectConfig(projectDir, ProjectConfig{
		KnownAgents: map[string]ProjectKnownAgent{
			"usr-aaa11111": {GlobalName: strPtr("alpha-alice")},
			"usr-bbb22222": {Name: strPtr("bob.1")},
		},
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	config, err := ReadProjectConfig(projectDir)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if config == nil {
		t.Fatal("expected config")
	}
	if config.ChannelID != "ch-11111111" {
		t.Fatalf("expected channel id set")
	}
	if config.KnownAgents["usr-aaa11111"].Name == nil || *config.KnownAgents["usr-aaa11111"].Name != "alice.1" {
		t.Fatalf("expected known agent name to persist")
	}
	if config.KnownAgents["usr-aaa11111"].GlobalName == nil || *config.KnownAgents["usr-aaa11111"].GlobalName != "alpha-alice" {
		t.Fatalf("expected global name to merge")
	}
	if config.KnownAgents["usr-bbb22222"].Name == nil || *config.KnownAgents["usr-bbb22222"].Name != "bob.1" {
		t.Fatalf("expected second agent")
	}
}

func TestReadMessagesSkipsMalformedLines(t *testing.T) {
	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(frayDir, messagesFile)

	good1, _ := json.Marshal(map[string]any{"type": "message", "id": "msg-good1", "mentions": []string{}})
	good2, _ := json.Marshal(map[string]any{"type": "message", "id": "msg-good2", "mentions": []string{}})
	contents := string(good1) + "\n" + "not-json\n" + string(good2) + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	readBack, err := ReadMessages(projectDir)
	if err != nil {
		t.Fatalf("read messages: %v", err)
	}
	if len(readBack) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(readBack))
	}
	if readBack[0].ID != "msg-good1" || readBack[1].ID != "msg-good2" {
		t.Fatalf("unexpected ids: %s, %s", readBack[0].ID, readBack[1].ID)
	}
}

func TestAppendAndReadQuestions(t *testing.T) {
	projectDir := t.TempDir()

	question := types.Question{
		GUID:      "qstn-abc12345",
		Re:        "target market?",
		FromAgent: "alice",
		Status:    types.QuestionStatusUnasked,
		CreatedAt: 123,
	}

	if err := AppendQuestion(projectDir, question); err != nil {
		t.Fatalf("append question: %v", err)
	}

	status := string(types.QuestionStatusOpen)
	askedIn := "msg-aaa11111"
	if err := AppendQuestionUpdate(projectDir, QuestionUpdateJSONLRecord{
		GUID:    question.GUID,
		Status:  &status,
		AskedIn: &askedIn,
	}); err != nil {
		t.Fatalf("append question update: %v", err)
	}

	readBack, err := ReadQuestions(projectDir)
	if err != nil {
		t.Fatalf("read questions: %v", err)
	}
	if len(readBack) != 1 {
		t.Fatalf("expected 1 question, got %d", len(readBack))
	}
	if readBack[0].Status != string(types.QuestionStatusOpen) {
		t.Fatalf("expected status open, got %s", readBack[0].Status)
	}
	if readBack[0].AskedIn == nil || *readBack[0].AskedIn != askedIn {
		t.Fatalf("expected asked_in to roundtrip")
	}
}

func TestReadThreadsEvents(t *testing.T) {
	projectDir := t.TempDir()

	thread := types.Thread{
		GUID:      "thrd-abc12345",
		Name:      "market-analysis",
		Status:    types.ThreadStatusOpen,
		CreatedAt: 123,
	}

	if err := AppendThread(projectDir, thread, []string{"alice", "bob"}); err != nil {
		t.Fatalf("append thread: %v", err)
	}

	status := string(types.ThreadStatusArchived)
	if err := AppendThreadUpdate(projectDir, ThreadUpdateJSONLRecord{
		GUID:   thread.GUID,
		Status: &status,
	}); err != nil {
		t.Fatalf("append thread update: %v", err)
	}

	if err := AppendThreadSubscribe(projectDir, ThreadSubscribeJSONLRecord{
		ThreadGUID:   thread.GUID,
		AgentID:      "charlie",
		SubscribedAt: 200,
	}); err != nil {
		t.Fatalf("append subscribe: %v", err)
	}

	if err := AppendThreadMessage(projectDir, ThreadMessageJSONLRecord{
		ThreadGUID:  thread.GUID,
		MessageGUID: "msg-aaa",
		AddedBy:     "alice",
		AddedAt:     300,
	}); err != nil {
		t.Fatalf("append thread message: %v", err)
	}

	threads, subs, msgs, err := ReadThreads(projectDir)
	if err != nil {
		t.Fatalf("read threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].Status != status {
		t.Fatalf("expected status archived, got %s", threads[0].Status)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription event, got %d", len(subs))
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 thread message event, got %d", len(msgs))
	}
}

func TestRebuildDatabaseFromJSONL(t *testing.T) {
	projectDir := t.TempDir()

	thread := types.Thread{
		GUID:      "thrd-abc12345",
		Name:      "analysis",
		Status:    types.ThreadStatusOpen,
		CreatedAt: 10,
	}

	if err := AppendThread(projectDir, thread, []string{"alice"}); err != nil {
		t.Fatalf("append thread: %v", err)
	}

	if err := AppendThreadSubscribe(projectDir, ThreadSubscribeJSONLRecord{
		ThreadGUID:   thread.GUID,
		AgentID:      "bob",
		SubscribedAt: 12,
	}); err != nil {
		t.Fatalf("append subscribe: %v", err)
	}

	if err := AppendThreadUnsubscribe(projectDir, ThreadUnsubscribeJSONLRecord{
		ThreadGUID:     thread.GUID,
		AgentID:        "bob",
		UnsubscribedAt: 13,
	}); err != nil {
		t.Fatalf("append unsubscribe: %v", err)
	}

	roomMessage := types.Message{
		ID:        "msg-aaaa1111",
		TS:        100,
		FromAgent: "alice",
		Body:      "room message",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
		Home:      "room",
	}
	threadMessage := types.Message{
		ID:        "msg-bbbb2222",
		TS:        110,
		FromAgent: "alice",
		Body:      "thread message",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
		Home:      thread.GUID,
	}

	if err := AppendMessage(projectDir, roomMessage); err != nil {
		t.Fatalf("append message: %v", err)
	}
	if err := AppendMessage(projectDir, threadMessage); err != nil {
		t.Fatalf("append message: %v", err)
	}

	if err := AppendThreadMessage(projectDir, ThreadMessageJSONLRecord{
		ThreadGUID:  thread.GUID,
		MessageGUID: roomMessage.ID,
		AddedBy:     "alice",
		AddedAt:     120,
	}); err != nil {
		t.Fatalf("append thread message: %v", err)
	}

	question := types.Question{
		GUID:      "qstn-abc12345",
		Re:        "target market?",
		FromAgent: "alice",
		Status:    types.QuestionStatusUnasked,
		CreatedAt: 200,
	}
	if err := AppendQuestion(projectDir, question); err != nil {
		t.Fatalf("append question: %v", err)
	}

	status := string(types.QuestionStatusAnswered)
	if err := AppendQuestionUpdate(projectDir, QuestionUpdateJSONLRecord{
		GUID:       question.GUID,
		Status:     &status,
		AnsweredIn: &roomMessage.ID,
	}); err != nil {
		t.Fatalf("append question update: %v", err)
	}

	dbConn := openTestDB(t)
	if err := RebuildDatabaseFromJSONL(dbConn, projectDir); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	rebuiltQuestion, err := GetQuestion(dbConn, question.GUID)
	if err != nil {
		t.Fatalf("get question: %v", err)
	}
	if rebuiltQuestion == nil || rebuiltQuestion.Status != types.QuestionStatusAnswered {
		t.Fatalf("expected answered question after rebuild")
	}
	if rebuiltQuestion.AnsweredIn == nil || *rebuiltQuestion.AnsweredIn != roomMessage.ID {
		t.Fatalf("expected answered_in to roundtrip")
	}

	messages, err := GetThreadMessages(dbConn, thread.GUID)
	if err != nil {
		t.Fatalf("get thread messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 thread messages, got %d", len(messages))
	}
	foundRoom := false
	foundThread := false
	for _, msg := range messages {
		switch msg.ID {
		case roomMessage.ID:
			foundRoom = true
		case threadMessage.ID:
			foundThread = true
		}
	}
	if !foundRoom || !foundThread {
		t.Fatalf("expected room and thread messages in thread view")
	}

	agentID := "alice"
	threads, err := GetThreads(dbConn, &types.ThreadQueryOptions{SubscribedAgent: &agentID})
	if err != nil {
		t.Fatalf("get subscribed threads: %v", err)
	}
	if len(threads) != 1 || threads[0].GUID != thread.GUID {
		t.Fatalf("expected alice subscribed to thread")
	}

	bobID := "bob"
	bobThreads, err := GetThreads(dbConn, &types.ThreadQueryOptions{SubscribedAgent: &bobID})
	if err != nil {
		t.Fatalf("get bob threads: %v", err)
	}
	if len(bobThreads) != 0 {
		t.Fatalf("expected bob unsubscribed after rebuild")
	}
}
