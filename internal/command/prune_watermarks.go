package command

import (
	"database/sql"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// fixStaleWatermarks updates agent watermarks that point to pruned messages.
// This prevents the daemon from getting "message not found" errors after pruning.
func fixStaleWatermarks(dbConn *sql.DB, projectPath string) error {
	agents, err := db.GetAllAgents(dbConn)
	if err != nil {
		return err
	}

	// Get latest message for fallback
	latestMsgs, err := db.GetMessages(dbConn, &types.MessageQueryOptions{Limit: 1})
	if err != nil {
		return err
	}
	var latestMsgID string
	if len(latestMsgs) > 0 {
		latestMsgID = latestMsgs[0].ID
	}

	for _, agent := range agents {
		if agent.MentionWatermark == nil || *agent.MentionWatermark == "" {
			continue
		}

		// Check if watermark message exists
		_, err := db.GetMessage(dbConn, *agent.MentionWatermark)
		if err == nil {
			continue // Message exists, watermark is valid
		}

		// Message doesn't exist - update watermark to latest
		newWatermark := latestMsgID
		if err := db.UpdateAgentWatermark(dbConn, agent.AgentID, newWatermark); err != nil {
			continue // Skip on error, daemon will self-heal
		}

		// Persist to JSONL
		db.AppendAgentUpdate(projectPath, db.AgentUpdateJSONLRecord{
			AgentID:          agent.AgentID,
			MentionWatermark: &newWatermark,
		})
	}

	return nil
}
