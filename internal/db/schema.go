package db

import (
	"database/sql"
	"fmt"

	"github.com/adamavenir/fray/internal/core"
)

// DBTX represents shared methods across sql.DB and sql.Tx.
type DBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// InitSchema initializes the fray schema.
func InitSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := initSchemaWith(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func initSchemaWith(db DBTX) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return err
	}
	if err := migrateSchema(db); err != nil {
		return err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return err
	}
	if _, err := db.Exec(defaultConfigSQL); err != nil {
		return err
	}
	return nil
}

// SchemaExists reports whether fray schema is present.
func SchemaExists(db *sql.DB) (bool, error) {
	row := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='fray_agents'
	`)
	var name string
	err := row.Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return name != "", nil
}

type tableColumn struct {
	Name    string
	ColType string
	NotNull int
	PK      int
}

func getTableInfo(db DBTX, table string) ([]tableColumn, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []tableColumn
	for rows.Next() {
		var col tableColumn
		var cid int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &col.Name, &col.ColType, &col.NotNull, &defaultValue, &col.PK); err != nil {
			return nil, err
		}
		columns = append(columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func hasColumn(columns []tableColumn, name string) bool {
	for _, col := range columns {
		if col.Name == name {
			return true
		}
	}
	return false
}

func hasPrimaryKey(columns []tableColumn, name string) bool {
	for _, col := range columns {
		if col.Name == name && col.PK > 0 {
			return true
		}
	}
	return false
}

func generateUniqueGUID(prefix string, used map[string]struct{}) (string, error) {
	for {
		guid, err := core.GenerateGUID(prefix)
		if err != nil {
			return "", err
		}
		if _, exists := used[guid]; !exists {
			used[guid] = struct{}{}
			return guid, nil
		}
	}
}

func migrateSchema(db DBTX) error {
	agentColumns, err := getTableInfo(db, "fray_agents")
	if err != nil {
		return err
	}

	if len(agentColumns) > 0 && hasColumn(agentColumns, "goal") && !hasColumn(agentColumns, "status") {
		if _, err := db.Exec("ALTER TABLE fray_agents RENAME COLUMN goal TO status"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE fray_agents RENAME COLUMN bio TO purpose"); err != nil {
			return err
		}
	}

	if len(agentColumns) > 0 && (!hasColumn(agentColumns, "guid") || !hasPrimaryKey(agentColumns, "guid")) {
		hasOldNames := hasColumn(agentColumns, "goal")
		statusCol := "status"
		purposeCol := "purpose"
		if hasOldNames {
			statusCol = "goal"
			purposeCol = "bio"
		}

		rows, err := db.Query(fmt.Sprintf(`
			SELECT agent_id, %s as status, %s as purpose, registered_at, last_seen, left_at
			FROM fray_agents
		`, statusCol, purposeCol))
		if err != nil {
			return err
		}
		defer rows.Close()

		type legacyAgent struct {
			AgentID      string
			Status       sql.NullString
			Purpose      sql.NullString
			RegisteredAt int64
			LastSeen     int64
			LeftAt       sql.NullInt64
		}
		var agents []legacyAgent
		for rows.Next() {
			var agent legacyAgent
			if err := rows.Scan(&agent.AgentID, &agent.Status, &agent.Purpose, &agent.RegisteredAt, &agent.LastSeen, &agent.LeftAt); err != nil {
				return err
			}
			agents = append(agents, agent)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if _, err := db.Exec(`
			CREATE TABLE fray_agents_new (
				guid TEXT PRIMARY KEY,
				agent_id TEXT NOT NULL UNIQUE,
				status TEXT,
				purpose TEXT,
				registered_at INTEGER NOT NULL,
				last_seen INTEGER NOT NULL,
				left_at INTEGER
			);
		`); err != nil {
			return err
		}

		used := make(map[string]struct{})
		for _, agent := range agents {
			guid, err := generateUniqueGUID("usr", used)
			if err != nil {
				return err
			}

			var status any
			if agent.Status.Valid {
				status = agent.Status.String
			}
			var purpose any
			if agent.Purpose.Valid {
				purpose = agent.Purpose.String
			}
			var leftAt any
			if agent.LeftAt.Valid {
				leftAt = agent.LeftAt.Int64
			}

			if _, err := db.Exec(
				`INSERT INTO fray_agents_new (guid, agent_id, status, purpose, registered_at, last_seen, left_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				guid, agent.AgentID, status, purpose, agent.RegisteredAt, agent.LastSeen, leftAt,
			); err != nil {
				return err
			}
		}

		if _, err := db.Exec("DROP TABLE fray_agents"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE fray_agents_new RENAME TO fray_agents"); err != nil {
			return err
		}
	}

	messageColumns, err := getTableInfo(db, "fray_messages")
	if err != nil {
		return err
	}
	needsMessageMigration := len(messageColumns) > 0 && (!hasColumn(messageColumns, "guid") ||
		!hasPrimaryKey(messageColumns, "guid") || !hasColumn(messageColumns, "reply_to") || !hasColumn(messageColumns, "channel_id"))

	idToGUID := make(map[int64]string)

	if needsMessageMigration {
		rows, err := db.Query(`
			SELECT id, ts, from_agent, body, mentions, type, reply_to, edited_at, archived_at
			FROM fray_messages
			ORDER BY id ASC
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		type legacyMessage struct {
			ID         int64
			TS         int64
			FromAgent  string
			Body       string
			Mentions   string
			MsgType    sql.NullString
			ReplyTo    sql.NullInt64
			EditedAt   sql.NullInt64
			ArchivedAt sql.NullInt64
		}
		var messages []legacyMessage
		for rows.Next() {
			var msg legacyMessage
			if err := rows.Scan(&msg.ID, &msg.TS, &msg.FromAgent, &msg.Body, &msg.Mentions, &msg.MsgType, &msg.ReplyTo, &msg.EditedAt, &msg.ArchivedAt); err != nil {
				return err
			}
			messages = append(messages, msg)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if _, err := db.Exec(`
			CREATE TABLE fray_messages_new (
				guid TEXT PRIMARY KEY,
				ts INTEGER NOT NULL,
				channel_id TEXT,
				home TEXT DEFAULT 'room',
				from_agent TEXT NOT NULL,
				origin TEXT,
				body TEXT NOT NULL,
				mentions TEXT NOT NULL DEFAULT '[]',
				type TEXT DEFAULT 'agent',
				"references" TEXT,
				surface_message TEXT,
				reply_to TEXT,
				edited_at INTEGER,
				archived_at INTEGER,
				reactions TEXT NOT NULL DEFAULT '{}'
			);
		`); err != nil {
			return err
		}

		used := make(map[string]struct{})
		for _, msg := range messages {
			guid, err := generateUniqueGUID("msg", used)
			if err != nil {
				return err
			}
			idToGUID[msg.ID] = guid
		}

		for _, msg := range messages {
			replyGUID := ""
			if msg.ReplyTo.Valid {
				if guid, ok := idToGUID[msg.ReplyTo.Int64]; ok {
					replyGUID = guid
				}
			}
			var replyValue any
			if replyGUID != "" {
				replyValue = replyGUID
			}

			msgType := "agent"
			if msg.MsgType.Valid && msg.MsgType.String != "" {
				msgType = msg.MsgType.String
			}

			var editedAt any
			if msg.EditedAt.Valid {
				editedAt = msg.EditedAt.Int64
			}
			var archivedAt any
			if msg.ArchivedAt.Valid {
				archivedAt = msg.ArchivedAt.Int64
			}

			if _, err := db.Exec(`
				INSERT INTO fray_messages_new (
					guid, ts, channel_id, home, from_agent, origin, body, mentions, type, "references", surface_message, reply_to, edited_at, archived_at, reactions
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				idToGUID[msg.ID],
				msg.TS,
				nil,
				"room",
				msg.FromAgent,
				nil,
				msg.Body,
				msg.Mentions,
				msgType,
				nil,
				nil,
				replyValue,
				editedAt,
				archivedAt,
				"{}",
			); err != nil {
				return err
			}
		}

		if _, err := db.Exec("DROP TABLE fray_messages"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE fray_messages_new RENAME TO fray_messages"); err != nil {
			return err
		}
	}
	if len(messageColumns) > 0 && !needsMessageMigration {
		if !hasColumn(messageColumns, "origin") {
			if _, err := db.Exec("ALTER TABLE fray_messages ADD COLUMN origin TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(messageColumns, "reactions") {
			if _, err := db.Exec("ALTER TABLE fray_messages ADD COLUMN reactions TEXT NOT NULL DEFAULT '{}'"); err != nil {
				return err
			}
		}
		if !hasColumn(messageColumns, "home") {
			if _, err := db.Exec("ALTER TABLE fray_messages ADD COLUMN home TEXT DEFAULT 'room'"); err != nil {
				return err
			}
		}
		if !hasColumn(messageColumns, "references") {
			if _, err := db.Exec("ALTER TABLE fray_messages ADD COLUMN \"references\" TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(messageColumns, "surface_message") {
			if _, err := db.Exec("ALTER TABLE fray_messages ADD COLUMN surface_message TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(messageColumns, "quote_message_guid") {
			if _, err := db.Exec("ALTER TABLE fray_messages ADD COLUMN quote_message_guid TEXT"); err != nil {
				return err
			}
		}
	}

	receiptColumns, err := getTableInfo(db, "fray_read_receipts")
	if err != nil {
		return err
	}
	if len(receiptColumns) > 0 && !hasColumn(receiptColumns, "message_guid") {
		rows, err := db.Query(`
			SELECT message_id, agent_prefix, read_at
			FROM fray_read_receipts
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		type legacyReceipt struct {
			MessageID   int64
			AgentPrefix string
			ReadAt      int64
		}
		var receipts []legacyReceipt
		for rows.Next() {
			var receipt legacyReceipt
			if err := rows.Scan(&receipt.MessageID, &receipt.AgentPrefix, &receipt.ReadAt); err != nil {
				return err
			}
			receipts = append(receipts, receipt)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if _, err := db.Exec(`
			CREATE TABLE fray_read_receipts_new (
				message_guid TEXT NOT NULL,
				agent_prefix TEXT NOT NULL,
				read_at INTEGER NOT NULL,
				PRIMARY KEY (message_guid, agent_prefix)
			);
		`); err != nil {
			return err
		}

		for _, receipt := range receipts {
			guid, ok := idToGUID[receipt.MessageID]
			if !ok {
				continue
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO fray_read_receipts_new (message_guid, agent_prefix, read_at) VALUES (?, ?, ?)`,
				guid, receipt.AgentPrefix, receipt.ReadAt,
			); err != nil {
				return err
			}
		}

		if _, err := db.Exec("DROP TABLE fray_read_receipts"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE fray_read_receipts_new RENAME TO fray_read_receipts"); err != nil {
			return err
		}
	}

	// Add options column to questions if missing
	questionColumns, err := getTableInfo(db, "fray_questions")
	if err != nil {
		return err
	}
	if len(questionColumns) > 0 && !hasColumn(questionColumns, "options") {
		if _, err := db.Exec("ALTER TABLE fray_questions ADD COLUMN options TEXT DEFAULT '[]'"); err != nil {
			return err
		}
	}

	// Add managed agent columns if missing
	agentColumns, err = getTableInfo(db, "fray_agents")
	if err != nil {
		return err
	}
	if len(agentColumns) > 0 {
		if !hasColumn(agentColumns, "managed") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN managed INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "invoke") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN invoke TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "presence") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN presence TEXT DEFAULT 'offline'"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "mention_watermark") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN mention_watermark TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "last_heartbeat") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN last_heartbeat INTEGER"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "reaction_watermark") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN reaction_watermark INTEGER"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "job_id") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN job_id TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "job_idx") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN job_idx INTEGER"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "is_ephemeral") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN is_ephemeral INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "aap_guid") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN aap_guid TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "presence_changed_at") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN presence_changed_at INTEGER"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "last_known_input") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN last_known_input INTEGER DEFAULT 0"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "last_known_output") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN last_known_output INTEGER DEFAULT 0"); err != nil {
				return err
			}
		}
		if !hasColumn(agentColumns, "tokens_updated_at") {
			if _, err := db.Exec("ALTER TABLE fray_agents ADD COLUMN tokens_updated_at INTEGER"); err != nil {
				return err
			}
		}
	}

	// Add thread anchor and activity columns if missing
	threadColumns, err := getTableInfo(db, "fray_threads")
	if err != nil {
		return err
	}
	if len(threadColumns) > 0 {
		if !hasColumn(threadColumns, "anchor_message_guid") {
			if _, err := db.Exec("ALTER TABLE fray_threads ADD COLUMN anchor_message_guid TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(threadColumns, "anchor_hidden") {
			if _, err := db.Exec("ALTER TABLE fray_threads ADD COLUMN anchor_hidden INTEGER NOT NULL DEFAULT 0"); err != nil {
				return err
			}
		}
		if !hasColumn(threadColumns, "last_activity_at") {
			if _, err := db.Exec("ALTER TABLE fray_threads ADD COLUMN last_activity_at INTEGER"); err != nil {
				return err
			}
		}
		if !hasColumn(threadColumns, "type") {
			if _, err := db.Exec("ALTER TABLE fray_threads ADD COLUMN type TEXT DEFAULT 'standard'"); err != nil {
				return err
			}
		}
	}

	// Add wake condition prompt columns if missing
	wakeColumns, err := getTableInfo(db, "fray_wake_conditions")
	if err != nil {
		return err
	}
	if len(wakeColumns) > 0 {
		if !hasColumn(wakeColumns, "prompt_text") {
			if _, err := db.Exec("ALTER TABLE fray_wake_conditions ADD COLUMN prompt_text TEXT"); err != nil {
				return err
			}
		}
		if !hasColumn(wakeColumns, "poll_interval_ms") {
			if _, err := db.Exec("ALTER TABLE fray_wake_conditions ADD COLUMN poll_interval_ms INTEGER"); err != nil {
				return err
			}
		}
		if !hasColumn(wakeColumns, "last_polled_at") {
			if _, err := db.Exec("ALTER TABLE fray_wake_conditions ADD COLUMN last_polled_at INTEGER"); err != nil {
				return err
			}
		}
	}

	return nil
}
