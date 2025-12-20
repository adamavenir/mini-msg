/**
 * Message type: user or agent.
 */
export type MessageType = 'user' | 'agent';

/**
 * Agent identity and presence.
 * Stored in mm_agents table.
 */
export interface Agent {
  guid: string;               // e.g., "usr-x9y8z7w6"
  agent_id: string;           // e.g., "alice.419", "pm.3.sub.1"
  goal: string | null;        // current focus/purpose
  bio: string | null;         // static identity info
  registered_at: number;      // unix timestamp
  last_seen: number;          // unix timestamp, updated on activity
  left_at: number | null;     // unix timestamp, null if active
}

/**
 * Room message.
 * Stored in mm_messages table.
 */
export interface Message {
  id: string;                 // GUID (msg-xxxxxxx)
  ts: number;                 // unix timestamp
  channel_id: string | null;  // channel GUID (nullable for legacy rows)
  from_agent: string;         // sender's full agent ID
  body: string;               // message content (markdown)
  mentions: string[];         // parsed from JSON, @mentioned agents
  type: MessageType;          // message type: 'user' or 'agent'
  reply_to: string | null;    // parent message guid for threading
  edited_at: number | null;   // unix timestamp of last edit, null if never edited
  archived_at: number | null; // unix timestamp of archival, null if not archived
}

/**
 * Raw message row from database.
 * mentions stored as JSON string.
 */
export interface MessageRow {
  guid: string;
  ts: number;
  channel_id: string | null;
  from_agent: string;
  body: string;
  mentions: string;           // JSON array as string
  type: MessageType;          // message type: 'user' or 'agent'
  reply_to: string | null;    // parent message guid for threading
  edited_at: number | null;   // unix timestamp of last edit, null if never edited
  archived_at: number | null; // unix timestamp of archival, null if not archived
}

/**
 * Linked project for cross-project messaging.
 * Stored in mm_linked_projects table.
 */
export interface LinkedProject {
  alias: string;              // short name for --project flag
  path: string;               // absolute path to project root
}

/**
 * Configuration setting.
 * Stored in mm_config table.
 */
export interface ConfigEntry {
  key: string;
  value: string;
}

/**
 * Parsed agent ID components.
 * Version is optional - simple names like "alice" or "eager-beaver" don't have versions.
 */
export interface ParsedAgentId {
  base: string;               // e.g., "alice", "pm.3.sub", "eager-beaver"
  version?: number;           // e.g., 419, 1 (undefined for simple names)
  full: string;               // original full ID
}

/**
 * Query options for message retrieval.
 */
export interface MessageQueryOptions {
  limit?: number;             // max messages to return
  since?: MessageCursor | string; // message cursor or GUID to start after
  before?: MessageCursor | string; // message cursor or GUID to end before
  filter?: Filter | null;     // filter to apply (null = no filter)
  unfiltered?: boolean;       // bypass saved filter (for explicit --unfiltered flag)
  unreadOnly?: boolean;       // only return unread messages (for read state tracking)
  agentPrefix?: string;       // agent prefix for read state filtering
  includeArchived?: boolean;  // include archived messages (default: false)
}

/**
 * Stable message cursor for ordering and paging.
 */
export interface MessageCursor {
  guid: string;
  ts: number;
}

/**
 * Agent filter preferences.
 * Stored in mm_filters table.
 */
export interface Filter {
  agent_id: string;
  mentions_pattern: string | null;
}

/**
 * Raw filter row from database.
 */
export interface FilterRow {
  agent_id: string;
  mentions_pattern: string | null;
}

/**
 * Read receipt for message tracking.
 * Stored in mm_read_receipts table.
 */
export interface ReadReceipt {
  message_guid: string;
  agent_prefix: string;    // base name without version (e.g., "alice")
  read_at: number;         // unix timestamp
}

/**
 * Claim type: file, beads issue, or GitHub issue.
 */
export type ClaimType = 'file' | 'bd' | 'issue';

/**
 * Resource claim for collision prevention.
 * Stored in mm_claims table.
 */
export interface Claim {
  id: number;
  agent_id: string;
  claim_type: ClaimType;
  pattern: string;          // file path/glob, bd id, or issue number
  reason: string | null;
  created_at: number;       // unix timestamp
  expires_at: number | null; // unix timestamp, null = no expiry
}

/**
 * Input for creating a claim (without id).
 */
export interface ClaimInput {
  agent_id: string;
  claim_type: ClaimType;
  pattern: string;
  reason?: string | null;
  expires_at?: number | null;
}
