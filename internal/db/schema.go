package db

import (
	"database/sql"
	"fmt"

	"github.com/adamavenir/fray/internal/core"
)

const schemaSQL = `
-- Agent presence and identity
CREATE TABLE IF NOT EXISTS fray_agents (
  guid TEXT PRIMARY KEY,               -- e.g., "usr-x9y8z7w6"
  agent_id TEXT NOT NULL UNIQUE,       -- e.g., "alice.419", "pm.3.sub.1"
  status TEXT,                         -- current task/focus (mutable)
  purpose TEXT,                        -- static identity/role info
  avatar TEXT,                         -- single-char avatar for display
  registered_at INTEGER NOT NULL,      -- unix timestamp
  last_seen INTEGER NOT NULL,          -- updated on post
  left_at INTEGER,                     -- set by "bye", null if active
  managed INTEGER NOT NULL DEFAULT 0,  -- whether daemon controls this agent
  invoke TEXT,                         -- JSON: driver config for spawning
  presence TEXT DEFAULT 'offline',     -- active, spawning, idle, error, offline
  mention_watermark TEXT,              -- last processed mention msg_id
  last_heartbeat INTEGER,              -- last silent checkin timestamp (ms)
  last_session_id TEXT                 -- Claude Code session UUID for --resume
);

-- Agent sessions (daemon-managed)
CREATE TABLE IF NOT EXISTS fray_agent_sessions (
  session_id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  triggered_by TEXT,                   -- msg_id that triggered spawn
  thread_guid TEXT,                    -- thread context if applicable
  started_at INTEGER NOT NULL,
  ended_at INTEGER,
  exit_code INTEGER,
  duration_ms INTEGER,
  FOREIGN KEY (agent_id) REFERENCES fray_agents(agent_id)
);

CREATE INDEX IF NOT EXISTS idx_fray_agent_sessions_agent ON fray_agent_sessions(agent_id);
CREATE INDEX IF NOT EXISTS idx_fray_agent_sessions_started ON fray_agent_sessions(started_at);

-- Room messages
CREATE TABLE IF NOT EXISTS fray_messages (
  guid TEXT PRIMARY KEY,               -- e.g., "msg-a1b2c3d4"
  ts INTEGER NOT NULL,                 -- unix timestamp
  channel_id TEXT,                     -- channel GUID for multi-channel support
  home TEXT DEFAULT 'room',            -- "room" or thread guid
  from_agent TEXT NOT NULL,            -- full agent address
  body TEXT NOT NULL,                  -- message content (markdown)
  mentions TEXT NOT NULL DEFAULT '[]', -- JSON array of mentioned addresses
  type TEXT DEFAULT 'agent',           -- 'user' or 'agent'
  "references" TEXT,                   -- referenced message guid (surface)
  surface_message TEXT,                -- surface message guid (backlink event)
  reply_to TEXT,                       -- parent message guid for threading
  quote_message_guid TEXT,             -- quoted message guid for inline quotes
  edited_at INTEGER,                   -- unix timestamp of last edit
  archived_at INTEGER,                 -- unix timestamp of archival
  reactions TEXT NOT NULL DEFAULT '{}' -- JSON object of reactions
);

CREATE INDEX IF NOT EXISTS idx_fray_messages_ts ON fray_messages(ts);
CREATE INDEX IF NOT EXISTS idx_fray_messages_from ON fray_messages(from_agent);
CREATE INDEX IF NOT EXISTS idx_fray_messages_archived ON fray_messages(archived_at);
CREATE INDEX IF NOT EXISTS idx_fray_messages_reply_to ON fray_messages(reply_to);

-- Questions
CREATE TABLE IF NOT EXISTS fray_questions (
  guid TEXT PRIMARY KEY,
  re TEXT NOT NULL,
  from_agent TEXT NOT NULL,
  to_agent TEXT,
  status TEXT DEFAULT 'unasked',
  thread_guid TEXT,
  asked_in TEXT,
  answered_in TEXT,
  options TEXT DEFAULT '[]',
  created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_fray_questions_status ON fray_questions(status);
CREATE INDEX IF NOT EXISTS idx_fray_questions_thread ON fray_questions(thread_guid);

-- Threads
CREATE TABLE IF NOT EXISTS fray_threads (
  guid TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  parent_thread TEXT,
  status TEXT DEFAULT 'open',
  type TEXT DEFAULT 'standard',
  created_at INTEGER NOT NULL,
  anchor_message_guid TEXT,
  anchor_hidden INTEGER NOT NULL DEFAULT 0,
  last_activity_at INTEGER,
  FOREIGN KEY (parent_thread) REFERENCES fray_threads(guid)
);

CREATE INDEX IF NOT EXISTS idx_fray_threads_parent ON fray_threads(parent_thread);
CREATE INDEX IF NOT EXISTS idx_fray_threads_status ON fray_threads(status);
CREATE INDEX IF NOT EXISTS idx_fray_threads_activity ON fray_threads(last_activity_at);
CREATE INDEX IF NOT EXISTS idx_fray_threads_type ON fray_threads(type);

-- Thread subscriptions
CREATE TABLE IF NOT EXISTS fray_thread_subscriptions (
  thread_guid TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  subscribed_at INTEGER NOT NULL,
  PRIMARY KEY (thread_guid, agent_id),
  FOREIGN KEY (thread_guid) REFERENCES fray_threads(guid)
);

-- Thread message membership (playlist)
CREATE TABLE IF NOT EXISTS fray_thread_messages (
  thread_guid TEXT NOT NULL,
  message_guid TEXT NOT NULL,
  added_by TEXT NOT NULL,
  added_at INTEGER NOT NULL,
  PRIMARY KEY (thread_guid, message_guid),
  FOREIGN KEY (thread_guid) REFERENCES fray_threads(guid)
);

CREATE INDEX IF NOT EXISTS idx_fray_thread_messages_message ON fray_thread_messages(message_guid);

-- Message pins (per-thread)
CREATE TABLE IF NOT EXISTS fray_message_pins (
  message_guid TEXT NOT NULL,
  thread_guid TEXT NOT NULL,
  pinned_by TEXT NOT NULL,
  pinned_at INTEGER NOT NULL,
  PRIMARY KEY (message_guid, thread_guid)
);

CREATE INDEX IF NOT EXISTS idx_fray_message_pins_thread ON fray_message_pins(thread_guid);

-- Thread pins (public, any agent can pin/unpin)
CREATE TABLE IF NOT EXISTS fray_thread_pins (
  thread_guid TEXT PRIMARY KEY,
  pinned_by TEXT NOT NULL,
  pinned_at INTEGER NOT NULL
);

-- Thread mutes (per-agent, with optional expiry)
CREATE TABLE IF NOT EXISTS fray_thread_mutes (
  thread_guid TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  muted_at INTEGER NOT NULL,
  expires_at INTEGER,
  PRIMARY KEY (thread_guid, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_fray_thread_mutes_agent ON fray_thread_mutes(agent_id);
CREATE INDEX IF NOT EXISTS idx_fray_thread_mutes_expires ON fray_thread_mutes(expires_at);

-- Linked projects for cross-project messaging
CREATE TABLE IF NOT EXISTS fray_linked_projects (
  alias TEXT PRIMARY KEY,
  path TEXT NOT NULL                     -- absolute path to .fray directory
);

-- Configuration
CREATE TABLE IF NOT EXISTS fray_config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- Agent filter preferences
CREATE TABLE IF NOT EXISTS fray_filters (
  agent_id TEXT PRIMARY KEY,
  mentions_pattern TEXT       -- comma-separated: "claude" or "claude,pm"
);

-- Read receipts for message tracking
CREATE TABLE IF NOT EXISTS fray_read_receipts (
  message_guid TEXT NOT NULL,
  agent_prefix TEXT NOT NULL,  -- base name without version (e.g., "alice" not "alice.1")
  read_at INTEGER NOT NULL,    -- unix timestamp
  PRIMARY KEY (message_guid, agent_prefix)
);

CREATE INDEX IF NOT EXISTS idx_fray_read_receipts_msg ON fray_read_receipts(message_guid);
CREATE INDEX IF NOT EXISTS idx_fray_read_receipts_agent ON fray_read_receipts(agent_prefix);

-- Watermark-based read tracking (replaces per-message receipts)
CREATE TABLE IF NOT EXISTS fray_read_to (
  agent_id TEXT NOT NULL,
  home TEXT NOT NULL,            -- "room" or thread GUID
  message_guid TEXT NOT NULL,
  message_ts INTEGER NOT NULL,
  set_at INTEGER NOT NULL,
  PRIMARY KEY (agent_id, home)
);

CREATE INDEX IF NOT EXISTS idx_fray_read_to_home ON fray_read_to(home);

-- Ghost cursors for session handoffs
CREATE TABLE IF NOT EXISTS fray_ghost_cursors (
  agent_id TEXT NOT NULL,
  home TEXT NOT NULL,            -- "room" or thread GUID
  message_guid TEXT NOT NULL,    -- start reading from here
  must_read INTEGER NOT NULL DEFAULT 0,  -- inject full content vs hint only
  set_at INTEGER NOT NULL,
  session_ack_at INTEGER,        -- when first viewed this session (null = not yet acked)
  PRIMARY KEY (agent_id, home)
);

CREATE INDEX IF NOT EXISTS idx_fray_ghost_cursors_agent ON fray_ghost_cursors(agent_id);

-- Resource claims for collision prevention
CREATE TABLE IF NOT EXISTS fray_claims (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent_id TEXT NOT NULL,
  claim_type TEXT NOT NULL,        -- 'file', 'bd', 'issue'
  pattern TEXT NOT NULL,           -- file path/glob, bd id, or issue number
  reason TEXT,
  created_at INTEGER NOT NULL,
  expires_at INTEGER,              -- null = no expiry
  UNIQUE(claim_type, pattern)
);

CREATE INDEX IF NOT EXISTS idx_fray_claims_agent ON fray_claims(agent_id);
CREATE INDEX IF NOT EXISTS idx_fray_claims_type ON fray_claims(claim_type);
CREATE INDEX IF NOT EXISTS idx_fray_claims_expires ON fray_claims(expires_at);

-- Reactions (not deduplicated, no remove)
CREATE TABLE IF NOT EXISTS fray_reactions (
  message_guid TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  emoji TEXT NOT NULL,
  reacted_at INTEGER NOT NULL,
  PRIMARY KEY (message_guid, agent_id, emoji, reacted_at)
);
CREATE INDEX IF NOT EXISTS idx_fray_reactions_message ON fray_reactions(message_guid);
CREATE INDEX IF NOT EXISTS idx_fray_reactions_agent ON fray_reactions(agent_id);

-- Faves (per-agent, polymorphic - threads or messages)
CREATE TABLE IF NOT EXISTS fray_faves (
  agent_id TEXT NOT NULL,
  item_type TEXT NOT NULL,  -- 'thread' | 'message'
  item_guid TEXT NOT NULL,
  faved_at INTEGER,         -- NULL if not faved, just has nickname
  nickname TEXT,            -- personal nickname for thread/message
  PRIMARY KEY (agent_id, item_type, item_guid)
);
CREATE INDEX IF NOT EXISTS idx_fray_faves_agent ON fray_faves(agent_id);
CREATE INDEX IF NOT EXISTS idx_fray_faves_item ON fray_faves(item_type, item_guid);

-- Role assignments (held roles - persistent)
CREATE TABLE IF NOT EXISTS fray_role_assignments (
  agent_id TEXT NOT NULL,
  role_name TEXT NOT NULL,
  assigned_at INTEGER NOT NULL,
  PRIMARY KEY (agent_id, role_name)
);
CREATE INDEX IF NOT EXISTS idx_fray_role_assignments_role ON fray_role_assignments(role_name);

-- Session roles (playing - cleared on bye/land)
CREATE TABLE IF NOT EXISTS fray_session_roles (
  agent_id TEXT NOT NULL,
  role_name TEXT NOT NULL,
  session_id TEXT,
  started_at INTEGER NOT NULL,
  PRIMARY KEY (agent_id, role_name)
);
CREATE INDEX IF NOT EXISTS idx_fray_session_roles_role ON fray_session_roles(role_name);
`

const defaultConfigSQL = `
INSERT OR IGNORE INTO fray_config (key, value) VALUES ('stale_hours', '4');
`

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
					guid, ts, channel_id, home, from_agent, body, mentions, type, "references", surface_message, reply_to, edited_at, archived_at, reactions
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				idToGUID[msg.ID],
				msg.TS,
				nil,
				"room",
				msg.FromAgent,
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

	return nil
}
