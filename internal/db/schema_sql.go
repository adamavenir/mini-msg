package db

const schemaSQL = `
-- Agent presence and identity
CREATE TABLE IF NOT EXISTS fray_agents (
  guid TEXT PRIMARY KEY,               -- e.g., "usr-x9y8z7w6"
  agent_id TEXT NOT NULL UNIQUE,       -- e.g., "alice.419", "pm.3.sub.1"
  aap_guid TEXT,                       -- AAP identity GUID (e.g., "aap-a1b2c3d4...")
  status TEXT,                         -- current task/focus (mutable)
  purpose TEXT,                        -- static identity/role info
  avatar TEXT,                         -- single-char avatar for display
  registered_at INTEGER NOT NULL,      -- unix timestamp
  last_seen INTEGER NOT NULL,          -- updated on post
  left_at INTEGER,                     -- set by "bye", null if active
  managed INTEGER NOT NULL DEFAULT 0,  -- whether daemon controls this agent
  invoke TEXT,                         -- JSON: driver config for spawning
  presence TEXT DEFAULT 'offline',     -- spawning/prompting/prompted/active (busy), idle (resumable), offline (left via bye), error
  presence_changed_at INTEGER,         -- when presence last changed (Unix ms, for TTL detection)
  mention_watermark TEXT,              -- last processed mention msg_id
  reaction_watermark INTEGER,          -- last processed reaction timestamp (ms)
  last_heartbeat INTEGER,              -- last silent checkin timestamp (ms)
  last_session_id TEXT,                -- Claude Code session UUID for --resume
  session_mode TEXT,                   -- "" (resumed), "n" (new), or 3-char fork prefix
  job_id TEXT,                         -- FK to fray_jobs.guid (null for regular agents)
  job_idx INTEGER,                     -- worker index within job (0-based)
  is_ephemeral INTEGER NOT NULL DEFAULT 0,  -- 1 for job workers
  last_known_input INTEGER DEFAULT 0,  -- token watermark: last seen input tokens
  last_known_output INTEGER DEFAULT 0, -- token watermark: last seen output tokens
  tokens_updated_at INTEGER            -- when token watermarks were last updated (Unix ms)
);

-- Agent descriptors (shared identity hints for onboarding)
CREATE TABLE IF NOT EXISTS fray_agent_descriptors (
  agent_id TEXT PRIMARY KEY,
  display_name TEXT,
  capabilities TEXT,                   -- JSON array
  updated_at INTEGER
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
  origin TEXT,                         -- machine id (multi-machine)
  session_id TEXT,                     -- session that posted this message
  body TEXT NOT NULL,                  -- message content (markdown)
  mentions TEXT NOT NULL DEFAULT '[]', -- JSON array of mentioned addresses
  fork_sessions TEXT,                  -- JSON object: agent → session ID for @agent#sessid spawns
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

-- Staged cursors (temporary, not persisted to JSONL)
-- Agents set these during a session, committed on bye/brb
CREATE TABLE IF NOT EXISTS fray_staged_cursors (
  agent_id TEXT NOT NULL,
  home TEXT NOT NULL,            -- "room" or thread GUID
  message_guid TEXT NOT NULL,    -- start reading from here
  must_read INTEGER NOT NULL DEFAULT 0,
  set_at INTEGER NOT NULL,
  PRIMARY KEY (agent_id, home)
);

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

-- Wake conditions (triggers for agent wake)
CREATE TABLE IF NOT EXISTS fray_wake_conditions (
  guid TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,              -- agent to wake
  set_by TEXT NOT NULL,                -- agent who set this condition
  type TEXT NOT NULL,                  -- 'on_mention', 'after', 'pattern', 'prompt'
  pattern TEXT,                        -- regex for pattern type
  on_agents TEXT DEFAULT '[]',         -- JSON array for on_mention type
  in_thread TEXT,                      -- scope to thread (null = anywhere except meta/)
  after_ms INTEGER,                    -- delay for after type
  use_router INTEGER NOT NULL DEFAULT 0, -- use haiku router for assessment
  prompt TEXT,                         -- context passed on wake
  prompt_text TEXT,                    -- LLM prompt for prompt type
  poll_interval_ms INTEGER,            -- poll interval for prompt type
  last_polled_at INTEGER,              -- last time prompt condition was polled
  persist_mode TEXT DEFAULT '',        -- '', 'persist', 'persist_until_bye', 'persist_restore_back'
  paused INTEGER NOT NULL DEFAULT 0,   -- true when paused (for restore-on-back)
  created_at INTEGER NOT NULL,
  expires_at INTEGER                   -- null = no expiry
);
CREATE INDEX IF NOT EXISTS idx_fray_wake_conditions_agent ON fray_wake_conditions(agent_id);
CREATE INDEX IF NOT EXISTS idx_fray_wake_conditions_type ON fray_wake_conditions(type);
CREATE INDEX IF NOT EXISTS idx_fray_wake_conditions_expires ON fray_wake_conditions(expires_at);

-- Jobs (parallel agent work units)
CREATE TABLE IF NOT EXISTS fray_jobs (
  guid TEXT PRIMARY KEY,               -- e.g., 'job-abc12345'
  name TEXT NOT NULL,
  context TEXT,                        -- JSON blob (issues, threads, messages, meta, instructions, refs)
  owner_agent TEXT,                    -- agent who created the job
  status TEXT DEFAULT 'running',       -- running/completed/failed/cancelled
  thread_guid TEXT,                    -- auto-created job-<id> thread
  created_at INTEGER NOT NULL,
  completed_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_fray_jobs_status ON fray_jobs(status);
`

const defaultConfigSQL = `
INSERT OR IGNORE INTO fray_config (key, value) VALUES ('stale_hours', '4');
`
