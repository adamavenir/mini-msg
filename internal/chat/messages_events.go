package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

func newEventMessage(body string) types.Message {
	return types.Message{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		TS:        time.Now().Unix(),
		FromAgent: "system",
		Body:      body,
		Type:      types.MessageTypeEvent,
	}
}

func filterUpdates(messages []types.Message, showUpdates bool) []types.Message {
	if showUpdates {
		return messages
	}
	filtered := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		// Filter out all event messages when showUpdates is false
		if msg.Type == types.MessageTypeEvent {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}

// isJoinLeaveEvent returns true if the message is a join/leave/rejoin event
func isJoinLeaveEvent(msg types.Message) bool {
	if msg.Type != types.MessageTypeEvent {
		return false
	}
	body := msg.Body
	// Check for the standard join/leave/rejoin patterns
	// These are: "@agent joined", "@agent rejoined", "@agent left"
	if strings.HasPrefix(body, "@") {
		if strings.HasSuffix(body, " joined") ||
			strings.HasSuffix(body, " rejoined") ||
			strings.HasSuffix(body, " left") {
			return true
		}
	}
	return false
}

// filterJoinLeaveEvents removes join/leave event messages while keeping other events
func filterJoinLeaveEvents(messages []types.Message) []types.Message {
	filtered := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if isJoinLeaveEvent(msg) {
			continue
		}
		filtered = append(filtered, msg)
	}
	return filtered
}
