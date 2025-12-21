package db

import (
	"database/sql"
	"fmt"

	"github.com/adamavenir/mini-msg/internal/core"
)

const schemaSQL = `
-- Agent presence and identity
CREATE TABLE IF NOT EXISTS mm_agents (
  guid TEXT PRIMARY KEY,               -- e.g., "usr-x9y8z7w6"
  agent_id TEXT NOT NULL UNIQUE,       -- e.g., "alice.419", "pm.3.sub.1"
  status TEXT,                         -- current task/focus (mutable)
  purpose TEXT,                        -- static identity/role info
  registered_at INTEGER NOT NULL,      -- unix timestamp
  last_seen INTEGER NOT NULL,          -- updated on post
  left_at INTEGER                      -- set by "bye", null if active
);

-- Room messages
CREATE TABLE IF NOT EXISTS mm_messages (
  guid TEXT PRIMARY KEY,               -- e.g., "msg-a1b2c3d4"
  ts INTEGER NOT NULL,                 -- unix timestamp
  channel_id TEXT,                     -- channel GUID for multi-channel support
  from_agent TEXT NOT NULL,            -- full agent address
  body TEXT NOT NULL,                  -- message content (markdown)
  mentions TEXT NOT NULL DEFAULT '[]', -- JSON array of mentioned addresses
  type TEXT DEFAULT 'agent',           -- 'user' or 'agent'
  reply_to TEXT,                       -- parent message guid for threading
  edited_at INTEGER,                   -- unix timestamp of last edit
  archived_at INTEGER                  -- unix timestamp of archival
);

CREATE INDEX IF NOT EXISTS idx_mm_messages_ts ON mm_messages(ts);
CREATE INDEX IF NOT EXISTS idx_mm_messages_from ON mm_messages(from_agent);
CREATE INDEX IF NOT EXISTS idx_mm_messages_archived ON mm_messages(archived_at);
CREATE INDEX IF NOT EXISTS idx_mm_messages_reply_to ON mm_messages(reply_to);

-- Linked projects for cross-project messaging
CREATE TABLE IF NOT EXISTS mm_linked_projects (
  alias TEXT PRIMARY KEY,
  path TEXT NOT NULL                     -- absolute path to .mm directory
);

-- Configuration
CREATE TABLE IF NOT EXISTS mm_config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- Agent filter preferences
CREATE TABLE IF NOT EXISTS mm_filters (
  agent_id TEXT PRIMARY KEY,
  mentions_pattern TEXT       -- comma-separated: "claude" or "claude,pm"
);

-- Read receipts for message tracking
CREATE TABLE IF NOT EXISTS mm_read_receipts (
  message_guid TEXT NOT NULL,
  agent_prefix TEXT NOT NULL,  -- base name without version (e.g., "alice" not "alice.1")
  read_at INTEGER NOT NULL,    -- unix timestamp
  PRIMARY KEY (message_guid, agent_prefix)
);

CREATE INDEX IF NOT EXISTS idx_mm_read_receipts_msg ON mm_read_receipts(message_guid);
CREATE INDEX IF NOT EXISTS idx_mm_read_receipts_agent ON mm_read_receipts(agent_prefix);

-- Resource claims for collision prevention
CREATE TABLE IF NOT EXISTS mm_claims (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent_id TEXT NOT NULL,
  claim_type TEXT NOT NULL,        -- 'file', 'bd', 'issue'
  pattern TEXT NOT NULL,           -- file path/glob, bd id, or issue number
  reason TEXT,
  created_at INTEGER NOT NULL,
  expires_at INTEGER,              -- null = no expiry
  UNIQUE(claim_type, pattern)
);

CREATE INDEX IF NOT EXISTS idx_mm_claims_agent ON mm_claims(agent_id);
CREATE INDEX IF NOT EXISTS idx_mm_claims_type ON mm_claims(claim_type);
CREATE INDEX IF NOT EXISTS idx_mm_claims_expires ON mm_claims(expires_at);
`

const defaultConfigSQL = `
INSERT OR IGNORE INTO mm_config (key, value) VALUES ('stale_hours', '4');
`

// DBTX represents shared methods across sql.DB and sql.Tx.
type DBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// InitSchema initializes the mm schema.
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

// SchemaExists reports whether mm schema is present.
func SchemaExists(db *sql.DB) (bool, error) {
	row := db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='mm_agents'
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
	agentColumns, err := getTableInfo(db, "mm_agents")
	if err != nil {
		return err
	}

	if len(agentColumns) > 0 && hasColumn(agentColumns, "goal") && !hasColumn(agentColumns, "status") {
		if _, err := db.Exec("ALTER TABLE mm_agents RENAME COLUMN goal TO status"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE mm_agents RENAME COLUMN bio TO purpose"); err != nil {
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
			FROM mm_agents
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
			CREATE TABLE mm_agents_new (
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
				`INSERT INTO mm_agents_new (guid, agent_id, status, purpose, registered_at, last_seen, left_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				guid, agent.AgentID, status, purpose, agent.RegisteredAt, agent.LastSeen, leftAt,
			); err != nil {
				return err
			}
		}

		if _, err := db.Exec("DROP TABLE mm_agents"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE mm_agents_new RENAME TO mm_agents"); err != nil {
			return err
		}
	}

	messageColumns, err := getTableInfo(db, "mm_messages")
	if err != nil {
		return err
	}
	needsMessageMigration := len(messageColumns) > 0 && (!hasColumn(messageColumns, "guid") ||
		!hasPrimaryKey(messageColumns, "guid") || !hasColumn(messageColumns, "reply_to") || !hasColumn(messageColumns, "channel_id"))

	idToGUID := make(map[int64]string)

	if needsMessageMigration {
		rows, err := db.Query(`
			SELECT id, ts, from_agent, body, mentions, type, reply_to, edited_at, archived_at
			FROM mm_messages
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
			CREATE TABLE mm_messages_new (
				guid TEXT PRIMARY KEY,
				ts INTEGER NOT NULL,
				channel_id TEXT,
				from_agent TEXT NOT NULL,
				body TEXT NOT NULL,
				mentions TEXT NOT NULL DEFAULT '[]',
				type TEXT DEFAULT 'agent',
				reply_to TEXT,
				edited_at INTEGER,
				archived_at INTEGER
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
				INSERT INTO mm_messages_new (
					guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				idToGUID[msg.ID],
				msg.TS,
				nil,
				msg.FromAgent,
				msg.Body,
				msg.Mentions,
				msgType,
				replyValue,
				editedAt,
				archivedAt,
			); err != nil {
				return err
			}
		}

		if _, err := db.Exec("DROP TABLE mm_messages"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE mm_messages_new RENAME TO mm_messages"); err != nil {
			return err
		}
	}

	receiptColumns, err := getTableInfo(db, "mm_read_receipts")
	if err != nil {
		return err
	}
	if len(receiptColumns) > 0 && !hasColumn(receiptColumns, "message_guid") {
		rows, err := db.Query(`
			SELECT message_id, agent_prefix, read_at
			FROM mm_read_receipts
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
			CREATE TABLE mm_read_receipts_new (
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
				`INSERT OR IGNORE INTO mm_read_receipts_new (message_guid, agent_prefix, read_at) VALUES (?, ?, ?)`,
				guid, receipt.AgentPrefix, receipt.ReadAt,
			); err != nil {
				return err
			}
		}

		if _, err := db.Exec("DROP TABLE mm_read_receipts"); err != nil {
			return err
		}
		if _, err := db.Exec("ALTER TABLE mm_read_receipts_new RENAME TO mm_read_receipts"); err != nil {
			return err
		}
	}

	return nil
}
