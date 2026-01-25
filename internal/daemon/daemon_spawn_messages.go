package daemon

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// runStdoutRepair evaluates captured stdout to determine if it should be posted.
// Returns nil if stdout-repair.mld doesn't exist (graceful degradation).
func (d *Daemon) runStdoutRepair(stdout string, lastPost *string, agentID string) *types.StdoutRepairResult {
	// Check if stdout-repair.mld exists (try new location, fall back to legacy)
	stdoutRepairPath := filepath.Join(d.project.Root, ".fray", "llm", "routers", "stdout-repair.mld")
	if _, err := os.Stat(stdoutRepairPath); os.IsNotExist(err) {
		// Try legacy location
		stdoutRepairPath = filepath.Join(d.project.Root, ".fray", "llm", "stdout-repair.mld")
		if _, err := os.Stat(stdoutRepairPath); os.IsNotExist(err) {
			d.debugf("stdout-repair: template not found, skipping")
			return nil
		}
	}

	// Build payload
	payload := types.StdoutRepairPayload{
		Stdout:   stdout,
		LastPost: lastPost,
		AgentID:  agentID,
	}

	// Encode payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		d.debugf("stdout-repair: marshal error: %v", err)
		return nil
	}

	// Run mlld with --payload flag
	cmd := exec.Command("mlld", "--payload", fmt.Sprintf("@payload=%s", string(payloadJSON)), stdoutRepairPath)
	cmd.Dir = d.project.Root

	output, err := cmd.Output()
	if err != nil {
		d.debugf("stdout-repair: mlld error: %v", err)
		return nil
	}

	// Parse result
	var result types.StdoutRepairResult
	if err := json.Unmarshal(output, &result); err != nil {
		d.debugf("stdout-repair: parse error: %v (output: %s)", err, output)
		return nil
	}

	return &result
}

// getMessagesAfter returns messages mentioning agent after the given watermark.
// Includes mentions in all threads (not just room) and replies to agent's messages.
func (d *Daemon) getMessagesAfter(watermark, agentID string) ([]types.Message, error) {
	// Empty string means all threads (room + threads)
	allHomes := ""
	opts := &types.MessageQueryOptions{
		Limit:                 100,
		Home:                  &allHomes,
		IncludeRepliesToAgent: agentID,
	}
	if watermark != "" {
		opts.SinceID = watermark
	}

	return db.GetMessagesWithMention(d.database, agentID, opts)
}

// getSessionMessages retrieves recent messages from a specific session for fork context.
// sessionID can be a prefix (e.g., "25912084") which will match full UUIDs like "25912084-8d46-497b-...".
func (d *Daemon) getSessionMessages(sessionID, agentID string, limit int) []types.Message {
	// Query messages where session_id matches prefix (supports @agent#sessid with short IDs)
	rows, err := d.database.Query(`
		SELECT guid, ts, channel_id, home, from_agent, session_id, body, mentions, fork_sessions, type, "references", surface_message, reply_to, quote_message_guid, edited_at, archived_at, reactions
		FROM fray_messages
		WHERE session_id LIKE ? || '%' AND from_agent = ?
		ORDER BY ts DESC
		LIMIT ?
	`, sessionID, agentID, limit)
	if err != nil {
		d.debugf("getSessionMessages error: %v", err)
		return nil
	}
	defer rows.Close()

	var messages []types.Message
	for rows.Next() {
		var msg types.Message
		var channelID, home, sessionIDVal, forkSessions, references, surfaceMessage, replyTo, quoteMsgGUID sql.NullString
		var editedAt, archivedAt sql.NullInt64
		var mentionsJSON, reactionsJSON string

		err := rows.Scan(
			&msg.ID, &msg.TS, &channelID, &home, &msg.FromAgent, &sessionIDVal,
			&msg.Body, &mentionsJSON, &forkSessions, &msg.Type,
			&references, &surfaceMessage, &replyTo, &quoteMsgGUID,
			&editedAt, &archivedAt, &reactionsJSON,
		)
		if err != nil {
			d.debugf("getSessionMessages scan error: %v", err)
			continue
		}

		if channelID.Valid {
			msg.ChannelID = &channelID.String
		}
		if home.Valid {
			msg.Home = home.String
		}
		if sessionIDVal.Valid {
			msg.SessionID = &sessionIDVal.String
		}
		if references.Valid {
			msg.References = &references.String
		}
		if surfaceMessage.Valid {
			msg.SurfaceMessage = &surfaceMessage.String
		}
		if replyTo.Valid {
			msg.ReplyTo = &replyTo.String
		}
		if quoteMsgGUID.Valid {
			msg.QuoteMessageGUID = &quoteMsgGUID.String
		}
		if editedAt.Valid {
			msg.EditedAt = &editedAt.Int64
		}
		if archivedAt.Valid {
			msg.ArchivedAt = &archivedAt.Int64
		}

		// Parse mentions JSON
		if mentionsJSON != "" {
			json.Unmarshal([]byte(mentionsJSON), &msg.Mentions)
		}
		// Parse fork_sessions JSON
		if forkSessions.Valid && forkSessions.String != "" {
			json.Unmarshal([]byte(forkSessions.String), &msg.ForkSessions)
		}

		messages = append(messages, msg)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages
}
