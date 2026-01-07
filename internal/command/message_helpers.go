package command

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func resolveMessageRef(dbConn *sql.DB, ref string) (*types.Message, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(ref, "#"))
	if trimmed == "" {
		return nil, fmt.Errorf("message reference is required")
	}

	msg, err := db.GetMessage(dbConn, trimmed)
	if err != nil {
		return nil, err
	}
	if msg != nil {
		return msg, nil
	}

	if !strings.HasPrefix(strings.ToLower(trimmed), "msg-") {
		msg, err = db.GetMessage(dbConn, "msg-"+trimmed)
		if err != nil {
			return nil, err
		}
		if msg != nil {
			return msg, nil
		}
	}

	msg, err = db.GetMessageByPrefix(dbConn, trimmed)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("message not found: %s", ref)
	}
	return msg, nil
}

// CollectQuotedMessages fetches all quoted messages for a list of messages.
// Returns a map of message ID -> quoted message for use in formatting.
func CollectQuotedMessages(dbConn *sql.DB, messages []types.Message) map[string]*types.Message {
	quotedMsgs := make(map[string]*types.Message)
	for _, msg := range messages {
		if msg.QuoteMessageGUID == nil {
			continue
		}
		quoteID := *msg.QuoteMessageGUID
		if _, exists := quotedMsgs[quoteID]; exists {
			continue
		}
		quoted, err := db.GetMessage(dbConn, quoteID)
		if err != nil || quoted == nil {
			continue
		}
		quotedMsgs[quoteID] = quoted
	}
	return quotedMsgs
}
