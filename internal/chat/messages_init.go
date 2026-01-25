package chat

import (
	"database/sql"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func loadInitialMessages(dbConn *sql.DB, projectDBPath string, limit int, includeArchived, showUpdates bool) ([]types.Message, []types.Message, *types.MessageCursor, *types.MessageCursor, error) {
	rawMessages, err := db.GetMessages(dbConn, &types.MessageQueryOptions{
		Limit:           limit,
		IncludeArchived: includeArchived,
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rawMessages, err = db.ApplyMessageEditCounts(projectDBPath, rawMessages)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	messages := filterUpdates(rawMessages, showUpdates)

	var lastCursor *types.MessageCursor
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		lastCursor = &types.MessageCursor{GUID: last.ID, TS: last.TS}
	}
	var oldestCursor *types.MessageCursor
	if len(rawMessages) > 0 {
		first := rawMessages[0]
		oldestCursor = &types.MessageCursor{GUID: first.ID, TS: first.TS}
	}

	return messages, rawMessages, lastCursor, oldestCursor, nil
}
