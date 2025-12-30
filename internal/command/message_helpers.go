package command

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
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
