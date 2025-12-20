import Database from 'better-sqlite3';
import { generateGuid } from '../core/guid.js';

const SCHEMA_SQL = `
-- Agent presence and identity
CREATE TABLE IF NOT EXISTS mm_agents (
  guid TEXT PRIMARY KEY,               -- e.g., "usr-x9y8z7w6"
  agent_id TEXT NOT NULL UNIQUE,       -- e.g., "alice.419", "pm.3.sub.1"
  goal TEXT,                           -- current focus/purpose (mutable)
  bio TEXT,                            -- static identity info (mutable)
  registered_at INTEGER NOT NULL,      -- unix timestamp
  last_seen INTEGER NOT NULL,          -- updated on post
  left_at INTEGER                      -- set by "bye", null if active
);

-- Room messages
CREATE TABLE IF NOT EXISTS mm_messages (
  guid TEXT PRIMARY KEY,               -- e.g., "msg-a1b2c3d4"
  ts INTEGER NOT NULL,                   -- unix timestamp
  channel_id TEXT,                       -- channel GUID for multi-channel support
  from_agent TEXT NOT NULL,              -- full agent address
  body TEXT NOT NULL,                    -- message content (markdown)
  mentions TEXT NOT NULL DEFAULT '[]',   -- JSON array of mentioned addresses
  type TEXT DEFAULT 'agent',             -- 'user' or 'agent'
  reply_to TEXT,                         -- parent message guid for threading
  edited_at INTEGER,                     -- unix timestamp of last edit
  archived_at INTEGER                    -- unix timestamp of archival
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
`;

const DEFAULT_CONFIG_SQL = `
-- Default config
INSERT OR IGNORE INTO mm_config (key, value) VALUES ('stale_hours', '4');
`;

/**
 * Initialize mm schema in database.
 * Safe to call multiple times (uses IF NOT EXISTS).
 */
export function initSchema(db: Database.Database): void {
  db.exec(SCHEMA_SQL);
  migrateSchema(db);
  db.exec(SCHEMA_SQL);
  db.exec(DEFAULT_CONFIG_SQL);
}

/**
 * Check if mm schema exists in database.
 */
export function schemaExists(db: Database.Database): boolean {
  const result = db.prepare(`
    SELECT name FROM sqlite_master
    WHERE type='table' AND name='mm_agents'
  `).get();

  return result !== undefined;
}

type TableColumn = { name: string; type: string; notnull: number; pk: number };

function getTableInfo(db: Database.Database, table: string): TableColumn[] {
  return db.prepare(`PRAGMA table_info(${table})`).all() as TableColumn[];
}

function hasColumn(columns: TableColumn[], name: string): boolean {
  return columns.some(col => col.name === name);
}

function hasPrimaryKey(columns: TableColumn[], name: string): boolean {
  return columns.some(col => col.name === name && col.pk > 0);
}

function migrateSchema(db: Database.Database): void {
  const migrate = db.transaction(() => {
    const agentColumns = getTableInfo(db, 'mm_agents');
    if (agentColumns.length > 0 && (!hasColumn(agentColumns, 'guid') || !hasPrimaryKey(agentColumns, 'guid'))) {
      const agents = db.prepare(`
        SELECT agent_id, goal, bio, registered_at, last_seen, left_at
        FROM mm_agents
      `).all() as {
        agent_id: string;
        goal: string | null;
        bio: string | null;
        registered_at: number;
        last_seen: number;
        left_at: number | null;
      }[];

      db.exec(`
        CREATE TABLE mm_agents_new (
          guid TEXT PRIMARY KEY,
          agent_id TEXT NOT NULL UNIQUE,
          goal TEXT,
          bio TEXT,
          registered_at INTEGER NOT NULL,
          last_seen INTEGER NOT NULL,
          left_at INTEGER
        );
      `);

      const insertAgent = db.prepare(`
        INSERT INTO mm_agents_new (guid, agent_id, goal, bio, registered_at, last_seen, left_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
      `);

      const usedGuids = new Set<string>();
      for (const agent of agents) {
        let guid = generateGuid('usr');
        while (usedGuids.has(guid)) {
          guid = generateGuid('usr');
        }
        usedGuids.add(guid);
        insertAgent.run(
          guid,
          agent.agent_id,
          agent.goal,
          agent.bio,
          agent.registered_at,
          agent.last_seen,
          agent.left_at
        );
      }

      db.exec('DROP TABLE mm_agents');
      db.exec('ALTER TABLE mm_agents_new RENAME TO mm_agents');
    }

    const messageColumns = getTableInfo(db, 'mm_messages');
    const needsMessageMigration =
      messageColumns.length > 0 &&
      (!hasColumn(messageColumns, 'guid') ||
        !hasPrimaryKey(messageColumns, 'guid') ||
        !hasColumn(messageColumns, 'reply_to') ||
        !hasColumn(messageColumns, 'channel_id'));

    let idToGuid = new Map<number, string>();

    if (needsMessageMigration) {
      const messages = db.prepare(`
        SELECT id, ts, from_agent, body, mentions, type, reply_to, edited_at, archived_at
        FROM mm_messages
        ORDER BY id ASC
      `).all() as {
        id: number;
        ts: number;
        from_agent: string;
        body: string;
        mentions: string;
        type: string | null;
        reply_to: number | null;
        edited_at: number | null;
        archived_at: number | null;
      }[];

      db.exec(`
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
      `);

      const insertMessage = db.prepare(`
        INSERT INTO mm_messages_new (
          guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `);

      const usedGuids = new Set<string>();
      for (const msg of messages) {
        let guid = generateGuid('msg');
        while (usedGuids.has(guid)) {
          guid = generateGuid('msg');
        }
        usedGuids.add(guid);
        idToGuid.set(msg.id, guid);
      }

      for (const msg of messages) {
        const replyGuid = msg.reply_to ? idToGuid.get(msg.reply_to) ?? null : null;
        insertMessage.run(
          idToGuid.get(msg.id),
          msg.ts,
          null,
          msg.from_agent,
          msg.body,
          msg.mentions,
          msg.type || 'agent',
          replyGuid,
          msg.edited_at,
          msg.archived_at
        );
      }

      db.exec('DROP TABLE mm_messages');
      db.exec('ALTER TABLE mm_messages_new RENAME TO mm_messages');
    }

    const receiptColumns = getTableInfo(db, 'mm_read_receipts');
    if (receiptColumns.length > 0 && !hasColumn(receiptColumns, 'message_guid')) {
      const receipts = db.prepare(`
        SELECT message_id, agent_prefix, read_at
        FROM mm_read_receipts
      `).all() as { message_id: number; agent_prefix: string; read_at: number }[];

      db.exec(`
        CREATE TABLE mm_read_receipts_new (
          message_guid TEXT NOT NULL,
          agent_prefix TEXT NOT NULL,
          read_at INTEGER NOT NULL,
          PRIMARY KEY (message_guid, agent_prefix)
        );
      `);

      const insertReceipt = db.prepare(`
        INSERT OR IGNORE INTO mm_read_receipts_new (message_guid, agent_prefix, read_at)
        VALUES (?, ?, ?)
      `);

      for (const receipt of receipts) {
        const guid = idToGuid.get(receipt.message_id);
        if (!guid) continue;
        insertReceipt.run(guid, receipt.agent_prefix, receipt.read_at);
      }

      db.exec('DROP TABLE mm_read_receipts');
      db.exec('ALTER TABLE mm_read_receipts_new RENAME TO mm_read_receipts');
    }
  });

  migrate();
}
