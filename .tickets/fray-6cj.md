---
id: fray-6cj
status: closed
deps: [fray-bml]
links: []
created: 2025-12-04T09:59:13.618061-08:00
type: task
priority: 1
parent: fray-08v
---
# Define TypeScript types for database entities

Create src/types.ts with TypeScript interfaces matching the database schema.

## Types to Define

```typescript
// src/types.ts

/**
 * Agent identity and presence.
 * Stored in bdm_agents table.
 */
export interface Agent {
  agent_id: string;           // e.g., "alice.419", "pm.3.sub.1"
  goal: string | null;        // current focus/purpose
  bio: string | null;         // static identity info
  registered_at: number;      // unix timestamp
  last_seen: number;          // unix timestamp, updated on activity
  left_at: number | null;     // unix timestamp, null if active
}

/**
 * Room message.
 * Stored in bdm_messages table.
 */
export interface Message {
  id: number;                 // sequential ID (1, 2, 3...)
  ts: number;                 // unix timestamp
  from_agent: string;         // sender's full agent ID
  body: string;               // message content (markdown)
  mentions: string[];         // parsed from JSON, @mentioned agents
}

/**
 * Raw message row from database.
 * mentions stored as JSON string.
 */
export interface MessageRow {
  id: number;
  ts: number;
  from_agent: string;
  body: string;
  mentions: string;           // JSON array as string
}

/**
 * Linked project for cross-project messaging.
 * Stored in bdm_linked_projects table.
 */
export interface LinkedProject {
  alias: string;              // short name for --project flag
  path: string;               // absolute path to project root
}

/**
 * Configuration setting.
 * Stored in bdm_config table.
 */
export interface ConfigEntry {
  key: string;
  value: string;
}

/**
 * Parsed agent ID components.
 */
export interface ParsedAgentId {
  base: string;               // e.g., "alice", "pm.3.sub"
  version: number;            // e.g., 419, 1
  full: string;               // original full ID
}

/**
 * Query options for message retrieval.
 */
export interface MessageQueryOptions {
  limit?: number;             // max messages to return
  since?: number;             // message ID to start after
  before?: number;            // message ID to end before
}
```

## Files
- src/types.ts

## Acceptance Criteria
- All types match database schema exactly
- Types are exported for use across codebase
- JSDoc comments explain each field
- MessageRow vs Message distinction is clear (raw vs parsed)


