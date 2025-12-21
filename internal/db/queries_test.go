package db

import (
	"strings"
	"testing"

	"github.com/adamavenir/mini-msg/internal/types"
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
