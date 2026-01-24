---
id: fray-1wj
status: closed
deps: [fray-wjm]
links: []
created: 2025-12-04T09:58:49.547065-08:00
type: task
priority: 1
parent: fray-08v
---
# Create database schema and initialization

Create src/db/schema.ts with SQL schema and initialization logic.

## Schema SQL
```sql
-- Agent presence and identity
CREATE TABLE IF NOT EXISTS bdm_agents (
  agent_id TEXT PRIMARY KEY,           -- e.g., "alice.419", "pm.3.sub.1"
  goal TEXT,                           -- current focus/purpose (mutable)
  bio TEXT,                            -- static identity info (mutable)
  registered_at INTEGER NOT NULL,      -- unix timestamp
  last_seen INTEGER NOT NULL,          -- updated on post
  left_at INTEGER                      -- set by "bye", null if active
);

-- Room messages
CREATE TABLE IF NOT EXISTS bdm_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,  -- sequential 1,2,3...
  ts INTEGER NOT NULL,                   -- unix timestamp
  from_agent TEXT NOT NULL,              -- full agent address
  body TEXT NOT NULL,                    -- message content (markdown)
  mentions TEXT NOT NULL DEFAULT '[]'    -- JSON array of mentioned addresses
);

CREATE INDEX IF NOT EXISTS idx_bdm_messages_ts ON bdm_messages(ts);
CREATE INDEX IF NOT EXISTS idx_bdm_messages_from ON bdm_messages(from_agent);

-- Linked projects for cross-project messaging
CREATE TABLE IF NOT EXISTS bdm_linked_projects (
  alias TEXT PRIMARY KEY,
  path TEXT NOT NULL                     -- absolute path to .beads directory
);

-- Configuration
CREATE TABLE IF NOT EXISTS bdm_config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- Default config
INSERT OR IGNORE INTO bdm_config (key, value) VALUES ('stale_hours', '4');
```

## API
```typescript
// src/db/schema.ts

/**
 * Initialize bdm schema in database.
 * Safe to call multiple times (uses IF NOT EXISTS).
 */
export function initSchema(db: Database): void;

/**
 * Check if bdm schema exists in database.
 */
export function schemaExists(db: Database): boolean;
```

## Files
- src/db/schema.ts

## Acceptance Criteria
- Schema creates all tables with correct structure
- Indexes are created for query performance
- Default config is inserted
- Idempotent - safe to run multiple times
- Unit tests verify table creation


