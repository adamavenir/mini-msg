package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func parseOptionalInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// ThreadActivityHint represents unread activity in a subscribed thread.
type ThreadActivityHint struct {
	ThreadGUID  string
	ThreadName  string
	NewCount    int
	LastMessage *types.Message `json:"-"` // Exclude from JSON to save tokens
	MustRead    bool           // from ghost cursor
}

// ThreadActivityHintJSON is a compact version for JSON output.
type ThreadActivityHintJSON struct {
	ThreadGUID   string `json:"thread_guid"`
	ThreadName   string `json:"thread_name"`
	NewCount     int    `json:"new_count"`
	LastMsgID    string `json:"last_msg_id,omitempty"`
	LastMsgFrom  string `json:"last_msg_from,omitempty"`
	LastMsgShort string `json:"last_msg_short,omitempty"` // First 80 chars
	MustRead     bool   `json:"must_read,omitempty"`
}

// toJSON converts a ThreadActivityHint to its compact JSON representation.
func (h *ThreadActivityHint) toJSON() ThreadActivityHintJSON {
	result := ThreadActivityHintJSON{
		ThreadGUID: h.ThreadGUID,
		ThreadName: h.ThreadName,
		NewCount:   h.NewCount,
		MustRead:   h.MustRead,
	}
	if h.LastMessage != nil {
		result.LastMsgID = h.LastMessage.ID
		result.LastMsgFrom = h.LastMessage.FromAgent
		body := h.LastMessage.Body
		if len(body) > 80 {
			body = body[:77] + "..."
		}
		result.LastMsgShort = body
	}
	return result
}

// getThreadActivityHints returns activity hints for subscribed threads.
func getThreadActivityHints(ctx *CommandContext, agentID string) ([]ThreadActivityHint, error) {
	// Get subscribed threads (excluding muted)
	threads, err := db.GetThreads(ctx.DB, &types.ThreadQueryOptions{
		SubscribedAgent: &agentID,
	})
	if err != nil {
		return nil, err
	}
	if len(threads) == 0 {
		return nil, nil
	}

	// Get muted threads to filter
	mutedGUIDs, err := db.GetMutedThreadGUIDs(ctx.DB, agentID)
	if err != nil {
		mutedGUIDs = map[string]bool{}
	}

	var hints []ThreadActivityHint
	for _, thread := range threads {
		if mutedGUIDs[thread.GUID] {
			continue
		}

		// Determine read position - check ghost cursor first, then read_to
		var sinceCursor *types.MessageCursor
		mustRead := false

		ghostCursor, _ := db.GetGhostCursor(ctx.DB, agentID, thread.GUID)
		if ghostCursor != nil {
			msg, err := db.GetMessage(ctx.DB, ghostCursor.MessageGUID)
			if err == nil && msg != nil {
				sinceCursor = &types.MessageCursor{GUID: msg.ID, TS: msg.TS}
				mustRead = ghostCursor.MustRead
			}
		}

		if sinceCursor == nil {
			readTo, _ := db.GetReadTo(ctx.DB, agentID, thread.GUID)
			if readTo != nil {
				sinceCursor = &types.MessageCursor{GUID: readTo.MessageGUID, TS: readTo.MessageTS}
			}
		}

		// Get messages since read position
		home := thread.GUID
		messages, err := db.GetMessages(ctx.DB, &types.MessageQueryOptions{
			Home:  &home,
			Since: sinceCursor,
		})
		if err != nil {
			continue
		}

		if len(messages) == 0 {
			continue
		}

		hint := ThreadActivityHint{
			ThreadGUID:  thread.GUID,
			ThreadName:  thread.Name,
			NewCount:    len(messages),
			LastMessage: &messages[len(messages)-1],
			MustRead:    mustRead,
		}
		hints = append(hints, hint)
	}

	return hints, nil
}

// formatThreadHint formats a single thread activity hint.
func formatThreadHint(hint ThreadActivityHint) string {
	suffix := ""
	if hint.MustRead {
		suffix = " [must-read]"
	}

	// Extract first line, truncate to ~30 chars
	context := ""
	if hint.LastMessage != nil {
		body := strings.TrimSpace(hint.LastMessage.Body)
		// Get first line only
		if idx := strings.Index(body, "\n"); idx > 0 {
			body = body[:idx]
		}
		body = strings.TrimSpace(body)
		if len(body) > 30 {
			body = body[:27] + "..."
		}
		if body != "" {
			context = fmt.Sprintf(" (last: @%s on %s)", hint.LastMessage.FromAgent, body)
		}
	}

	return fmt.Sprintf("  %s: %d new%s%s", hint.ThreadName, hint.NewCount, context, suffix)
}

// isDirectMention checks if the message body starts with @agent (direct address).
func isDirectMention(body, agentBase string) bool {
	body = strings.TrimSpace(body)
	// Check for @agent or @agent.* at start
	if strings.HasPrefix(body, "@"+agentBase+" ") || strings.HasPrefix(body, "@"+agentBase+"\n") {
		return true
	}
	if strings.HasPrefix(body, "@"+agentBase+".") {
		// Could be @agent.1 or @agent.something
		return true
	}
	if body == "@"+agentBase {
		return true
	}
	return false
}
