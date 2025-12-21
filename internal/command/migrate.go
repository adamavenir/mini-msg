package command

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

type tableColumn struct {
	Name    string
	Type    string
	NotNull int
	PK      int
}

type agentRow struct {
	GUID         *string
	AgentID      string
	Goal         *string
	Bio          *string
	RegisteredAt int64
	LastSeen     int64
	LeftAt       *int64
}

type messageRow struct {
	GUID       *string
	ID         *int64
	TS         int64
	FromAgent  string
	Body       string
	Mentions   *string
	Type       *string
	ReplyTo    any
	EditedAt   *int64
	ArchivedAt *int64
}

type readReceiptRow struct {
	MessageGUID *string
	MessageID   *int64
	AgentPrefix string
	ReadAt      int64
}

// NewMigrateCmd creates the migrate command.
func NewMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate mm project from v0.1.0 to v0.2.0 format",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := core.DiscoverProject("")
			if err != nil {
				return writeCommandError(cmd, err)
			}

			mmDir := filepath.Dir(project.DBPath)
			configPath := filepath.Join(mmDir, "mm-config.json")

			if _, err := os.Stat(configPath); err == nil {
				return writeCommandError(cmd, fmt.Errorf("mm-config.json already exists. Nothing to migrate."))
			}

			backupDir := filepath.Join(project.Root, ".mm.bak")
			if _, err := os.Stat(backupDir); err == nil {
				return writeCommandError(cmd, fmt.Errorf("Backup already exists at .mm.bak/. Move it aside before migrating."))
			}

			defaultName := filepath.Base(project.Root)
			channelName, err := promptMigrateChannelName(defaultName)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			channelID, err := core.GenerateGUID("ch")
			if err != nil {
				return writeCommandError(cmd, err)
			}

			sourceDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", project.DBPath))
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer sourceDB.Close()

			if !tableExists(sourceDB, "mm_agents") || !tableExists(sourceDB, "mm_messages") {
				return writeCommandError(cmd, fmt.Errorf("Missing mm tables in database. Nothing to migrate."))
			}

			agentColumns, err := getColumns(sourceDB, "mm_agents")
			if err != nil {
				return writeCommandError(cmd, err)
			}
			hasAgentGUID := columnsInclude(agentColumns, "guid")
			agents, err := loadAgents(sourceDB, hasAgentGUID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			messageColumns, err := getColumns(sourceDB, "mm_messages")
			if err != nil {
				return writeCommandError(cmd, err)
			}
			hasMessageGUID := columnsInclude(messageColumns, "guid")
			hasMessageID := columnsInclude(messageColumns, "id")
			messages, err := loadMessages(sourceDB, hasMessageGUID, hasMessageID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			readReceipts, err := loadReadReceipts(sourceDB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			usedAgentGuids := map[string]struct{}{}
			knownAgents := map[string]db.ProjectKnownAgent{}
			agentsJSONL := make([]db.AgentJSONLRecord, 0, len(agents))

			for _, agent := range agents {
				guid := agent.GUID
				if guid == nil || *guid == "" || containsGUID(usedAgentGuids, *guid) {
					generated, err := generateUniqueGUID("usr", usedAgentGuids)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					guid = &generated
				} else {
					usedAgentGuids[*guid] = struct{}{}
				}

				createdAt := time.Unix(agent.RegisteredAt, 0).UTC().Format(time.RFC3339)
				status := "active"
				if agent.LeftAt != nil {
					status = "inactive"
				}
				globalName := agent.AgentID
				if channelName != "" {
					globalName = fmt.Sprintf("%s-%s", channelName, agent.AgentID)
				}

				knownAgents[*guid] = db.ProjectKnownAgent{
					Name:        &agent.AgentID,
					GlobalName:  &globalName,
					HomeChannel: &channelID,
					CreatedAt:   &createdAt,
					Status:      &status,
				}

				agentsJSONL = append(agentsJSONL, db.AgentJSONLRecord{
					Type:         "agent",
					ID:           *guid,
					Name:         agent.AgentID,
					GlobalName:   &globalName,
					HomeChannel:  &channelID,
					CreatedAt:    &createdAt,
					ActiveStatus: nil,
					AgentID:      agent.AgentID,
					Status:       &status,
					Purpose:      nil,
					Goal:         agent.Goal,
					Bio:          agent.Bio,
					RegisteredAt: agent.RegisteredAt,
					LastSeen:     agent.LastSeen,
					LeftAt:       agent.LeftAt,
				})
			}

			if !hasMessageGUID && !hasMessageID {
				return writeCommandError(cmd, fmt.Errorf("Could not locate message IDs for migration."))
			}

			usedMessageGuids := map[string]struct{}{}
			idToGuid := map[int64]string{}
			messageGuids := make([]string, len(messages))

			for i, message := range messages {
				guid := message.GUID
				if guid == nil || *guid == "" || containsGUID(usedMessageGuids, *guid) {
					generated, err := generateUniqueGUID("msg", usedMessageGuids)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					messageGuids[i] = generated
				} else {
					messageGuids[i] = *guid
					usedMessageGuids[*guid] = struct{}{}
				}
				if message.ID != nil {
					idToGuid[*message.ID] = messageGuids[i]
				}
			}

			messagesJSONL := make([]db.MessageJSONLRecord, 0, len(messages))
			for i, message := range messages {
				replyTo := resolveReplyTo(message.ReplyTo, hasMessageGUID, idToGuid)
				messageType := types.MessageTypeAgent
				if message.Type != nil && *message.Type == string(types.MessageTypeUser) {
					messageType = types.MessageTypeUser
				}

				messagesJSONL = append(messagesJSONL, db.MessageJSONLRecord{
					Type:       "message",
					ID:         messageGuids[i],
					ChannelID:  &channelID,
					FromAgent:  message.FromAgent,
					Body:       message.Body,
					Mentions:   parseMentions(message.Mentions),
					MsgType:    messageType,
					ReplyTo:    replyTo,
					TS:         message.TS,
					EditedAt:   message.EditedAt,
					ArchivedAt: message.ArchivedAt,
				})
			}

			if err := copyDir(mmDir, backupDir); err != nil {
				return writeCommandError(cmd, err)
			}

			_, err = db.UpdateProjectConfig(project.DBPath, db.ProjectConfig{
				Version:     1,
				ChannelID:   channelID,
				ChannelName: channelName,
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
				KnownAgents: knownAgents,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := writeJSONLFile(filepath.Join(mmDir, "agents.jsonl"), agentsJSONL); err != nil {
				return writeCommandError(cmd, err)
			}
			if err := writeJSONLFile(filepath.Join(mmDir, "messages.jsonl"), messagesJSONL); err != nil {
				return writeCommandError(cmd, err)
			}

			targetDB, err := sql.Open("sqlite", project.DBPath)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if _, err := targetDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}
			if _, err := targetDB.Exec("PRAGMA journal_mode = WAL"); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}
			if _, err := targetDB.Exec("PRAGMA busy_timeout = 5000"); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}

			if err := db.RebuildDatabaseFromJSONL(targetDB, project.DBPath); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}

			if err := restoreReadReceipts(targetDB, readReceipts, idToGuid); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}
			_ = targetDB.Close()

			if _, err := core.RegisterChannel(channelID, channelName, project.Root); err != nil {
				return writeCommandError(cmd, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Registered channel %s as '%s'\n", channelID, channelName)
			fmt.Fprintln(cmd.OutOrStdout(), "Migration complete. Backup at .mm.bak/")
			fmt.Fprintf(cmd.OutOrStdout(), "Migrated %d agents and %d messages.\n", len(agentsJSONL), len(messagesJSONL))
			return nil
		},
	}

	return cmd
}

func promptMigrateChannelName(defaultName string) (string, error) {
	if !isTTY(os.Stdin) {
		return defaultName, nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stdout, "Channel name for this project? [%s]: ", defaultName)
	text, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return defaultName, nil
	}
	return trimmed, nil
}

func tableExists(dbConn *sql.DB, table string) bool {
	row := dbConn.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name=?
	`, table)
	var name string
	if err := row.Scan(&name); err != nil {
		return false
	}
	return name != ""
}

func getColumns(dbConn *sql.DB, table string) ([]tableColumn, error) {
	rows, err := dbConn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []tableColumn
	for rows.Next() {
		var col tableColumn
		var cid int
		var defaultValue any
		if err := rows.Scan(&cid, &col.Name, &col.Type, &col.NotNull, &defaultValue, &col.PK); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func columnsInclude(columns []tableColumn, name string) bool {
	for _, col := range columns {
		if col.Name == name {
			return true
		}
	}
	return false
}

func loadAgents(dbConn *sql.DB, hasGUID bool) ([]agentRow, error) {
	if hasGUID {
		rows, err := dbConn.Query(`
			SELECT guid, agent_id, goal, bio, registered_at, last_seen, left_at
			FROM mm_agents
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanAgents(rows, true)
	}

	rows, err := dbConn.Query(`
		SELECT agent_id, goal, bio, registered_at, last_seen, left_at
		FROM mm_agents
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAgents(rows, false)
}

func scanAgents(rows *sql.Rows, hasGUID bool) ([]agentRow, error) {
	var agents []agentRow
	for rows.Next() {
		var (
			guid         sql.NullString
			agentID      string
			goal         sql.NullString
			bio          sql.NullString
			registeredAt int64
			lastSeen     int64
			leftAt       sql.NullInt64
		)
		if hasGUID {
			if err := rows.Scan(&guid, &agentID, &goal, &bio, &registeredAt, &lastSeen, &leftAt); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&agentID, &goal, &bio, &registeredAt, &lastSeen, &leftAt); err != nil {
				return nil, err
			}
		}

		agents = append(agents, agentRow{
			GUID:         nullStringPtr(guid),
			AgentID:      agentID,
			Goal:         nullStringPtr(goal),
			Bio:          nullStringPtr(bio),
			RegisteredAt: registeredAt,
			LastSeen:     lastSeen,
			LeftAt:       nullInt64Ptr(leftAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return agents, nil
}

func loadMessages(dbConn *sql.DB, hasGUID bool, hasID bool) ([]messageRow, error) {
	fields := []string{}
	if hasGUID {
		fields = append(fields, "guid")
	}
	if hasID {
		fields = append(fields, "id")
	}
	fields = append(fields, "ts", "from_agent", "body", "mentions", "type", "reply_to", "edited_at", "archived_at")
	query := fmt.Sprintf(`
		SELECT %s
		FROM mm_messages
		ORDER BY %s
	`, strings.Join(fields, ", "), messageOrder(hasID))

	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMessages(rows, hasGUID, hasID)
}

func messageOrder(hasID bool) string {
	if hasID {
		return "ts ASC, id ASC"
	}
	return "ts ASC"
}

func scanMessages(rows *sql.Rows, hasGUID bool, hasID bool) ([]messageRow, error) {
	var messages []messageRow
	for rows.Next() {
		var (
			guid       sql.NullString
			id         sql.NullInt64
			ts         int64
			fromAgent  string
			body       string
			mentions   sql.NullString
			msgType    sql.NullString
			replyTo    any
			editedAt   sql.NullInt64
			archivedAt sql.NullInt64
		)

		switch {
		case hasGUID && hasID:
			if err := rows.Scan(&guid, &id, &ts, &fromAgent, &body, &mentions, &msgType, &replyTo, &editedAt, &archivedAt); err != nil {
				return nil, err
			}
		case hasGUID && !hasID:
			if err := rows.Scan(&guid, &ts, &fromAgent, &body, &mentions, &msgType, &replyTo, &editedAt, &archivedAt); err != nil {
				return nil, err
			}
		case !hasGUID && hasID:
			if err := rows.Scan(&id, &ts, &fromAgent, &body, &mentions, &msgType, &replyTo, &editedAt, &archivedAt); err != nil {
				return nil, err
			}
		default:
			if err := rows.Scan(&ts, &fromAgent, &body, &mentions, &msgType, &replyTo, &editedAt, &archivedAt); err != nil {
				return nil, err
			}
		}

		messages = append(messages, messageRow{
			GUID:       nullStringPtr(guid),
			ID:         nullInt64Ptr(id),
			TS:         ts,
			FromAgent:  fromAgent,
			Body:       body,
			Mentions:   nullStringPtr(mentions),
			Type:       nullStringPtr(msgType),
			ReplyTo:    replyTo,
			EditedAt:   nullInt64Ptr(editedAt),
			ArchivedAt: nullInt64Ptr(archivedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func loadReadReceipts(dbConn *sql.DB) ([]readReceiptRow, error) {
	if !tableExists(dbConn, "mm_read_receipts") {
		return nil, nil
	}

	columns, err := getColumns(dbConn, "mm_read_receipts")
	if err != nil {
		return nil, err
	}
	hasGUID := columnsInclude(columns, "message_guid")

	field := "message_id"
	if hasGUID {
		field = "message_guid"
	}

	rows, err := dbConn.Query(fmt.Sprintf(`
		SELECT %s, agent_prefix, read_at
		FROM mm_read_receipts
	`, field))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var receipts []readReceiptRow
	for rows.Next() {
		var (
			messageGUID sql.NullString
			messageID   sql.NullInt64
			agentPrefix string
			readAt      int64
		)
		if hasGUID {
			if err := rows.Scan(&messageGUID, &agentPrefix, &readAt); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&messageID, &agentPrefix, &readAt); err != nil {
				return nil, err
			}
		}
		receipts = append(receipts, readReceiptRow{
			MessageGUID: nullStringPtr(messageGUID),
			MessageID:   nullInt64Ptr(messageID),
			AgentPrefix: agentPrefix,
			ReadAt:      readAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func restoreReadReceipts(dbConn *sql.DB, receipts []readReceiptRow, idToGuid map[int64]string) error {
	if len(receipts) == 0 {
		return nil
	}

	tx, err := dbConn.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO mm_read_receipts (message_guid, agent_prefix, read_at)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, receipt := range receipts {
		messageGUID := receipt.MessageGUID
		if messageGUID == nil && receipt.MessageID != nil {
			if guid, ok := idToGuid[*receipt.MessageID]; ok {
				messageGUID = &guid
			}
		}
		if messageGUID == nil {
			continue
		}
		if _, err := stmt.Exec(*messageGUID, receipt.AgentPrefix, receipt.ReadAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func parseMentions(raw *string) []string {
	if raw == nil || *raw == "" {
		return []string{}
	}
	var parsed []any
	if err := json.Unmarshal([]byte(*raw), &parsed); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(parsed))
	for _, entry := range parsed {
		if value, ok := entry.(string); ok && value != "" {
			out = append(out, value)
		}
	}
	return out
}

func generateUniqueGUID(prefix string, used map[string]struct{}) (string, error) {
	for {
		guid, err := core.GenerateGUID(prefix)
		if err != nil {
			return "", err
		}
		if _, exists := used[guid]; exists {
			continue
		}
		used[guid] = struct{}{}
		return guid, nil
	}
}

func containsGUID(used map[string]struct{}, guid string) bool {
	_, exists := used[guid]
	return exists
}

func resolveReplyTo(value any, hasMessageGUID bool, idToGuid map[int64]string) *string {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case int64:
		if guid, ok := idToGuid[v]; ok {
			return &guid
		}
		return nil
	case []byte:
		text := strings.TrimSpace(string(v))
		return resolveReplyString(text, hasMessageGUID, idToGuid)
	case string:
		return resolveReplyString(strings.TrimSpace(v), hasMessageGUID, idToGuid)
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", v))
		return resolveReplyString(text, hasMessageGUID, idToGuid)
	}
}

func resolveReplyString(value string, hasMessageGUID bool, idToGuid map[int64]string) *string {
	if value == "" {
		return nil
	}
	if !hasMessageGUID {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			if guid, ok := idToGuid[parsed]; ok {
				return &guid
			}
			return nil
		}
	}
	return &value
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func writeJSONLFile(filePath string, records any) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}

	var lines []string
	switch v := records.(type) {
	case []db.AgentJSONLRecord:
		lines = make([]string, 0, len(v))
		for _, record := range v {
			row, err := json.Marshal(record)
			if err != nil {
				return err
			}
			lines = append(lines, string(row))
		}
	case []db.MessageJSONLRecord:
		lines = make([]string, 0, len(v))
		for _, record := range v {
			row, err := json.Marshal(record)
			if err != nil {
				return err
			}
			lines = append(lines, string(row))
		}
	default:
		return fmt.Errorf("unsupported JSONL record type")
	}

	if len(lines) == 0 {
		return os.WriteFile(filePath, []byte(""), 0o644)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filePath, []byte(content), 0o644)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
