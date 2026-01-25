package chat

import (
	"database/sql"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func initReadWatermarks(dbConn *sql.DB, username string, messages []types.Message, threads []types.Thread) *types.MessageCursor {
	// Initialize mention cursor from persisted watermark to avoid re-notifying
	var lastMentionCursor *types.MessageCursor
	if username != "" {
		if mentionWatermark, _ := db.GetReadTo(dbConn, username, "mentions"); mentionWatermark != nil {
			lastMentionCursor = &types.MessageCursor{GUID: mentionWatermark.MessageGUID, TS: mentionWatermark.MessageTS}
		}
	}

	// Initialize read watermarks if not set - for human users, everything should be
	// "read" when starting fray, only new messages arriving after startup are unread
	if username != "" && len(messages) > 0 {
		latest := messages[len(messages)-1]
		// Initialize room watermark if not set
		if roomWatermark, _ := db.GetReadTo(dbConn, username, ""); roomWatermark == nil {
			_ = db.SetReadTo(dbConn, username, "", latest.ID, latest.TS)
		}
		// Initialize thread watermarks for subscribed threads that don't have one
		for _, thread := range threads {
			if threadWatermark, _ := db.GetReadTo(dbConn, username, thread.GUID); threadWatermark == nil {
				// Get latest message in thread and set watermark
				threadMsgs, err := db.GetMessages(dbConn, &types.MessageQueryOptions{
					Home:  &thread.GUID,
					Limit: 1,
				})
				if err == nil && len(threadMsgs) > 0 {
					last := threadMsgs[len(threadMsgs)-1]
					_ = db.SetReadTo(dbConn, username, thread.GUID, last.ID, last.TS)
				}
			}
		}
	}

	return lastMentionCursor
}
