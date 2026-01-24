package db

import (
	"strings"
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

func TestCreateAndGetMessage(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	created, err := CreateMessage(db, types.Message{
		FromAgent: "alice.1",
		Body:      "hello",
		Mentions:  []string{"bob.1"},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	fetched, err := GetMessage(db, created.ID)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected message")
	}
	if fetched.Body != "hello" {
		t.Fatalf("unexpected body: %s", fetched.Body)
	}
}

func TestGetMessagesWithLimitAndCursor(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	msg1, err := CreateMessage(db, types.Message{
		TS:        100,
		FromAgent: "alice.1",
		Body:      "one",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message 1: %v", err)
	}
	msg2, err := CreateMessage(db, types.Message{
		TS:        200,
		FromAgent: "alice.1",
		Body:      "two",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message 2: %v", err)
	}
	msg3, err := CreateMessage(db, types.Message{
		TS:        300,
		FromAgent: "alice.1",
		Body:      "three",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message 3: %v", err)
	}

	messages, err := GetMessages(db, &types.MessageQueryOptions{Limit: 2})
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].ID != msg2.ID || messages[1].ID != msg3.ID {
		t.Fatalf("expected last two messages")
	}

	messages, err = GetMessages(db, &types.MessageQueryOptions{SinceID: msg1.ID})
	if err != nil {
		t.Fatalf("get messages since: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after cursor, got %d", len(messages))
	}
	if messages[0].ID != msg2.ID || messages[1].ID != msg3.ID {
		t.Fatalf("unexpected messages after cursor")
	}
}

func TestGetMessagesWithMentionUnread(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	msg1, err := CreateMessage(db, types.Message{
		FromAgent: "alice.1",
		Body:      "hi @bob",
		Mentions:  []string{"bob"},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	_, err = CreateMessage(db, types.Message{
		FromAgent: "alice.1",
		Body:      "ping @all",
		Mentions:  []string{"all"},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	messages, err := GetMessagesWithMention(db, "bob", nil)
	if err != nil {
		t.Fatalf("get messages with mention: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages for mention, got %d", len(messages))
	}

	if err := MarkMessagesRead(db, []string{msg1.ID}, "bob"); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	messages, err = GetMessagesWithMention(db, "bob", &types.MessageQueryOptions{UnreadOnly: true})
	if err != nil {
		t.Fatalf("get unread mentions: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 unread message, got %d", len(messages))
	}
}

func TestGetDistinctOriginsForAgent(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	_, err := CreateMessage(db, types.Message{
		FromAgent: "alice",
		Origin:    "laptop",
		Body:      "hello",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	_, err = CreateMessage(db, types.Message{
		FromAgent: "alice",
		Origin:    "server",
		Body:      "second",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	_, err = CreateMessage(db, types.Message{
		FromAgent: "alice",
		Body:      "no origin",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	_, err = CreateMessage(db, types.Message{
		FromAgent: "bob",
		Origin:    "laptop",
		Body:      "other",
		Mentions:  []string{},
		Type:      types.MessageTypeAgent,
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}

	origins, err := GetDistinctOriginsForAgent(db, "alice")
	if err != nil {
		t.Fatalf("get origins: %v", err)
	}
	if len(origins) != 2 {
		t.Fatalf("expected 2 origins, got %d", len(origins))
	}
	if !(origins[0] == "laptop" && origins[1] == "server") && !(origins[0] == "server" && origins[1] == "laptop") {
		t.Fatalf("unexpected origins: %v", origins)
	}
}

func TestClaimsConflicts(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	claim, err := CreateClaim(db, types.ClaimInput{
		AgentID:   "alice.1",
		ClaimType: types.ClaimTypeFile,
		Pattern:   "src/*.go",
	})
	if err != nil {
		t.Fatalf("create claim: %v", err)
	}
	if claim == nil {
		t.Fatal("expected claim")
	}

	_, err = CreateClaim(db, types.ClaimInput{
		AgentID:   "bob.1",
		ClaimType: types.ClaimTypeFile,
		Pattern:   "src/*.go",
	})
	if err == nil {
		t.Fatal("expected duplicate claim error")
	}
	if !strings.Contains(err.Error(), "already claimed") {
		t.Fatalf("unexpected error: %v", err)
	}

	conflicts, err := FindConflictingFileClaims(db, []string{"src/main.go"}, "")
	if err != nil {
		t.Fatalf("find conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
}

func TestCreateAndUpdateQuestion(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	question, err := CreateQuestion(db, types.Question{
		Re:        "target market?",
		FromAgent: "alice",
		Status:    types.QuestionStatusUnasked,
	})
	if err != nil {
		t.Fatalf("create question: %v", err)
	}

	status := string(types.QuestionStatusAnswered)
	answerID := "msg-abc12345"
	updated, err := UpdateQuestion(db, question.GUID, QuestionUpdates{
		Status:     types.OptionalString{Set: true, Value: &status},
		AnsweredIn: types.OptionalString{Set: true, Value: &answerID},
	})
	if err != nil {
		t.Fatalf("update question: %v", err)
	}
	if updated.Status != types.QuestionStatusAnswered {
		t.Fatalf("expected answered status")
	}
	if updated.AnsweredIn == nil || *updated.AnsweredIn != answerID {
		t.Fatalf("expected answered_in set")
	}
}

func TestThreadMessagesIncludeHomeAndMembership(t *testing.T) {
	db := openTestDB(t)
	requireSchema(t, db)

	thread, err := CreateThread(db, types.Thread{
		Name:   "analysis",
		Status: types.ThreadStatusOpen,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	roomMsg, err := CreateMessage(db, types.Message{
		FromAgent: "alice",
		Body:      "room message",
		Mentions:  []string{},
	})
	if err != nil {
		t.Fatalf("create room message: %v", err)
	}
	threadMsg, err := CreateMessage(db, types.Message{
		FromAgent: "alice",
		Body:      "thread message",
		Mentions:  []string{},
		Home:      thread.GUID,
	})
	if err != nil {
		t.Fatalf("create thread message: %v", err)
	}

	if err := AddMessageToThread(db, thread.GUID, roomMsg.ID, "alice", 0); err != nil {
		t.Fatalf("add message to thread: %v", err)
	}

	messages, err := GetThreadMessages(db, thread.GUID)
	if err != nil {
		t.Fatalf("get thread messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].ID != roomMsg.ID && messages[1].ID != roomMsg.ID {
		t.Fatalf("expected room message in thread")
	}
	if messages[0].ID != threadMsg.ID && messages[1].ID != threadMsg.ID {
		t.Fatalf("expected thread message in thread")
	}
}
