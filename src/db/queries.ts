import type { Agent, Message, MessageRow, MessageQueryOptions, MessageCursor, LinkedProject, ConfigEntry, MessageType, Filter, FilterRow, ReadReceipt, Claim, ClaimInput, ClaimType } from '../types.js';
import type Database from 'better-sqlite3';
import micromatch from 'micromatch';
import { generateGuid } from '../core/guid.js';

/**
 * Get agent by exact ID.
 * @returns Agent or undefined if not found
 */
export function getAgent(db: Database.Database, agentId: string): Agent | undefined {
  const stmt = db.prepare('SELECT * FROM mm_agents WHERE agent_id = ?');
  return stmt.get(agentId) as Agent | undefined;
}

/**
 * Get agents matching a prefix.
 * Used for @mention resolution and "mm who" command.
 * @example getAgentsByPrefix(db, "alice") -> [alice.1, alice.419, alice.frontend.3]
 * @example getAgentsByPrefix(db, "alice.frontend") -> [alice.frontend.1, alice.frontend.3]
 */
export function getAgentsByPrefix(db: Database.Database, prefix: string): Agent[] {
  const stmt = db.prepare(`
    SELECT * FROM mm_agents
    WHERE agent_id = ? OR agent_id LIKE ?
    ORDER BY agent_id
  `);
  return stmt.all(prefix, `${prefix}.%`) as Agent[];
}

/**
 * Create new agent.
 * @throws if agent_id already exists
 */
export function createAgent(
  db: Database.Database,
  agent: Omit<Agent, 'guid' | 'left_at'> & { guid?: string }
): void {
  const guid = agent.guid ?? generateUniqueGuid(db, 'mm_agents', 'usr');
  const stmt = db.prepare(`
    INSERT INTO mm_agents (guid, agent_id, status, purpose, registered_at, last_seen, left_at)
    VALUES (?, ?, ?, ?, ?, ?, NULL)
  `);
  stmt.run(guid, agent.agent_id, agent.status, agent.purpose, agent.registered_at, agent.last_seen);
}

/**
 * Update agent fields.
 * Only updates provided fields.
 */
export function updateAgent(
  db: Database.Database,
  agentId: string,
  updates: Partial<Pick<Agent, 'status' | 'purpose' | 'last_seen' | 'left_at'>>
): void {
  const fields: string[] = [];
  const values: any[] = [];

  if (updates.status !== undefined) {
    fields.push('status = ?');
    values.push(updates.status);
  }
  if (updates.purpose !== undefined) {
    fields.push('purpose = ?');
    values.push(updates.purpose);
  }
  if (updates.last_seen !== undefined) {
    fields.push('last_seen = ?');
    values.push(updates.last_seen);
  }
  if (updates.left_at !== undefined) {
    fields.push('left_at = ?');
    values.push(updates.left_at);
  }

  if (fields.length === 0) return;

  values.push(agentId);

  const sql = `UPDATE mm_agents SET ${fields.join(', ')} WHERE agent_id = ?`;
  const stmt = db.prepare(sql);
  stmt.run(...values);
}

/**
 * Get active agents (not left, not stale).
 * @param staleHours - hours of inactivity before considered stale
 */
export function getActiveAgents(db: Database.Database, staleHours: number): Agent[] {
  const stmt = db.prepare(`
    SELECT * FROM mm_agents
    WHERE left_at IS NULL
      AND last_seen > (strftime('%s', 'now') - ? * 3600)
    ORDER BY last_seen DESC
  `);
  return stmt.all(staleHours) as Agent[];
}

/**
 * Get all agents including stale and left.
 */
export function getAllAgents(db: Database.Database): Agent[] {
  const stmt = db.prepare('SELECT * FROM mm_agents ORDER BY agent_id');
  return stmt.all() as Agent[];
}

/**
 * Get active users (humans using mm chat) based on recent messages.
 * @param staleHours - Hours of inactivity before considered inactive
 * @returns Array of usernames who have posted type='user' messages recently
 */
export function getActiveUsers(db: Database.Database, staleHours: number): string[] {
  const stmt = db.prepare(`
    SELECT from_agent, MAX(ts) as last_ts
    FROM mm_messages
    WHERE type = 'user'
      AND ts > (strftime('%s', 'now') - ? * 3600)
    GROUP BY from_agent
    ORDER BY last_ts DESC
  `);
  const rows = stmt.all(staleHours) as { from_agent: string; last_ts: number }[];
  return rows.map(r => r.from_agent);
}

/**
 * Get all unique agent base names from the database.
 * For simple names (alice, eager-beaver), returns the full name.
 * For legacy IDs (alice.1), extracts the base (alice).
 */
export function getAgentBases(db: Database.Database): Set<string> {
  const stmt = db.prepare('SELECT agent_id FROM mm_agents');
  const rows = stmt.all() as { agent_id: string }[];
  const bases = new Set<string>();

  for (const row of rows) {
    const id = row.agent_id;
    // Check if it's a legacy format (ends with .N where N is a number)
    const lastDotIndex = id.lastIndexOf('.');
    if (lastDotIndex !== -1) {
      const suffix = id.substring(lastDotIndex + 1);
      const version = parseInt(suffix, 10);
      if (Number.isInteger(version) && version > 0 && version.toString() === suffix) {
        // Legacy format - extract base
        bases.add(id.substring(0, lastDotIndex));
        continue;
      }
    }
    // Simple format - use full ID as base
    bases.add(id);
  }

  return bases;
}

/**
 * Check if an agent is currently active (not left, not stale).
 * @param db - Database instance
 * @param agentId - Agent ID to check
 * @param staleHours - Hours of inactivity before considered stale
 * @returns true if agent exists and is active
 */
export function isAgentActive(db: Database.Database, agentId: string, staleHours: number): boolean {
  const stmt = db.prepare(`
    SELECT 1 FROM mm_agents
    WHERE agent_id = ?
      AND left_at IS NULL
      AND last_seen > (strftime('%s', 'now') - ? * 3600)
  `);
  const result = stmt.get(agentId, staleHours);
  return result !== undefined;
}

/**
 * Rename an agent.
 * Updates agent_id in mm_agents and from_agent in mm_messages.
 * @param db - Database instance
 * @param oldId - Current agent ID
 * @param newId - New agent ID
 * @throws if old agent doesn't exist or new ID already taken
 */
export function renameAgent(db: Database.Database, oldId: string, newId: string): void {
  // Check old agent exists
  const oldAgent = getAgent(db, oldId);
  if (!oldAgent) {
    throw new Error(`Agent not found: ${oldId}`);
  }

  // Check new ID not already taken
  const existingAgent = getAgent(db, newId);
  if (existingAgent) {
    throw new Error(`Agent already exists: ${newId}`);
  }

  // Update agent_id in mm_agents
  const updateAgentStmt = db.prepare('UPDATE mm_agents SET agent_id = ? WHERE agent_id = ?');
  updateAgentStmt.run(newId, oldId);

  // Update from_agent in mm_messages
  const updateMessagesStmt = db.prepare('UPDATE mm_messages SET from_agent = ? WHERE from_agent = ?');
  updateMessagesStmt.run(newId, oldId);

  // Update mentions in mm_messages (stored as JSON array)
  // This is more complex - need to update JSON arrays that contain the old ID
  const messagesWithMention = db.prepare(`
    SELECT guid, mentions FROM mm_messages
    WHERE mentions LIKE ?
  `).all(`%"${oldId}"%`) as { guid: string; mentions: string }[];

  for (const row of messagesWithMention) {
    const mentions = JSON.parse(row.mentions) as string[];
    const updatedMentions = mentions.map(m => m === oldId ? newId : m);
    db.prepare('UPDATE mm_messages SET mentions = ? WHERE guid = ?')
      .run(JSON.stringify(updatedMentions), row.guid);
  }

  // Update claims agent_id
  updateClaimsAgentId(db, oldId, newId);
}

/**
 * Get highest version number for a base name.
 * Used by "mm new" to auto-increment.
 * @example getMaxVersion(db, "alice") -> 419 (if alice.419 exists)
 * @returns 0 if no agents with this base exist
 */
export function getMaxVersion(db: Database.Database, base: string): number {
  // Use GLOB to match base.N where N is numeric
  // GLOB is case-sensitive, [0-9]* matches any sequence of digits
  const pattern = `${base}.[0-9]*`;
  const stmt = db.prepare('SELECT agent_id FROM mm_agents WHERE agent_id GLOB ?');
  const rows = stmt.all(pattern) as { agent_id: string }[];

  if (rows.length === 0) {
    return 0;
  }

  // Parse out version numbers and find max
  let maxVersion = 0;
  for (const row of rows) {
    // Extract the last numeric segment
    const parts = row.agent_id.split('.');
    const lastPart = parts[parts.length - 1];
    const version = parseInt(lastPart, 10);
    if (!isNaN(version) && version > maxVersion) {
      maxVersion = version;
    }
  }

  return maxVersion;
}

/**
 * Get next version number for a base name.
 * Used by "mm new" to auto-increment.
 * @example nextVersion(db, "alice") -> 1 (if no alice agents exist)
 * @example nextVersion(db, "alice") -> 420 (if alice.419 exists)
 */
export function nextVersion(db: Database.Database, base: string): number {
  return getMaxVersion(db, base) + 1;
}

function generateUniqueGuid(
  db: Database.Database,
  table: 'mm_agents' | 'mm_messages',
  prefix: string
): string {
  for (let attempt = 0; attempt < 5; attempt++) {
    const guid = generateGuid(prefix);
    const exists = db.prepare(`SELECT 1 FROM ${table} WHERE guid = ?`).get(guid);
    if (!exists) {
      return guid;
    }
  }

  throw new Error(`Failed to generate unique ${prefix} GUID`);
}

// Message functions

/**
 * Convert MessageRow to Message (parse mentions JSON).
 */
export function parseMessageRow(row: MessageRow): Message {
  return {
    id: row.guid,
    ts: row.ts,
    channel_id: row.channel_id,
    from_agent: row.from_agent,
    body: row.body,
    mentions: JSON.parse(row.mentions),
    type: row.type,
    reply_to: row.reply_to,
    edited_at: row.edited_at,
    archived_at: row.archived_at,
  };
}

/**
 * Create a new message.
 * @param message - Message to create (ts will be auto-set)
 * @param mentions - Pre-extracted mentions array (caller extracts from body)
 * @returns The created message with ID
 */
export function createMessage(
  db: Database.Database,
  message: {
    ts?: number;
    channel_id?: string | null;
    from_agent: string;
    body: string;
    mentions: string[];
    type?: MessageType;
    reply_to?: string;
  }
): Message {
  const ts = message.ts ?? Math.floor(Date.now() / 1000);
  const channelId = message.channel_id ?? getConfig(db, 'channel_id') ?? null;
  const mentionsJson = JSON.stringify(message.mentions);
  const type = message.type ?? 'agent';
  const replyTo = message.reply_to ?? null;
  const guid = generateUniqueGuid(db, 'mm_messages', 'msg');

  const stmt = db.prepare(`
    INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)
  `);

  stmt.run(guid, ts, channelId, message.from_agent, message.body, mentionsJson, type, replyTo);

  return {
    id: guid,
    ts,
    channel_id: channelId,
    from_agent: message.from_agent,
    body: message.body,
    mentions: message.mentions,
    type,
    reply_to: replyTo,
    edited_at: null,
    archived_at: null,
  };
}

function resolveCursor(
  db: Database.Database,
  cursor?: MessageCursor | string
): MessageCursor | undefined {
  if (!cursor) return undefined;
  if (typeof cursor !== 'string') return cursor;

  const row = db.prepare('SELECT guid, ts FROM mm_messages WHERE guid = ?').get(cursor) as {
    guid: string;
    ts: number;
  } | undefined;

  return row ? { guid: row.guid, ts: row.ts } : undefined;
}

function buildCursorCondition(
  prefix: string,
  operator: '>' | '<',
  cursor: MessageCursor
): { clause: string; params: any[] } {
  const tsCol = `${prefix}ts`;
  const guidCol = `${prefix}guid`;
  return {
    clause: `(${tsCol} ${operator} ? OR (${tsCol} = ? AND ${guidCol} ${operator} ?))`,
    params: [cursor.ts, cursor.ts, cursor.guid],
  };
}

/**
 * Get messages with optional filtering.
 * Returns in chronological order (oldest first).
 */
export function getMessages(
  db: Database.Database,
  options?: MessageQueryOptions
): Message[] {
  const sinceCursor = resolveCursor(db, options?.since);
  if (options?.since && !sinceCursor) {
    throw new Error(`Message not found: ${options.since}`);
  }

  const beforeCursor = resolveCursor(db, options?.before);
  if (options?.before && !beforeCursor) {
    throw new Error(`Message not found: ${options.before}`);
  }

  // Build filter conditions if filter is provided
  const buildFilterConditions = (filter: Filter | null | undefined): string[] => {
    if (!filter?.mentions_pattern) return [];

    return [`EXISTS (
      SELECT 1 FROM json_each(mentions)
      WHERE value LIKE '${filter.mentions_pattern.replace(/'/g, "''")}'
    )`];
  };

  // If we have a limit but no since/before, we want the LAST N messages
  // This requires a subquery to get the last N by (ts, guid), then order them chronologically
  if (options?.limit !== undefined && !sinceCursor && !beforeCursor) {
    const conditions: string[] = [];

    // Exclude archived messages by default
    if (options?.includeArchived !== true) {
      conditions.push("archived_at IS NULL");
    }

    const filterConditions = buildFilterConditions(options.filter);
    conditions.push(...filterConditions);

    let whereClause = '';
    if (conditions.length > 0) {
      whereClause = ' WHERE ' + conditions.join(' AND ');
    }

    const sql = `
      SELECT * FROM (
        SELECT * FROM mm_messages${whereClause}
        ORDER BY ts DESC, guid DESC
        LIMIT ?
      ) ORDER BY ts ASC, guid ASC
    `;
    const stmt = db.prepare(sql);
    const rows = stmt.all(options.limit) as MessageRow[];
    return rows.map(parseMessageRow);
  }

  // Regular query for other cases
  let sql = 'SELECT * FROM mm_messages';
  const params: any[] = [];

  const conditions: string[] = [];

  // Exclude archived messages by default
  if (options?.includeArchived !== true) {
    conditions.push('archived_at IS NULL');
  }

  if (sinceCursor) {
    const clause = buildCursorCondition('', '>', sinceCursor);
    conditions.push(clause.clause);
    params.push(...clause.params);
  }

  if (beforeCursor) {
    const clause = buildCursorCondition('', '<', beforeCursor);
    conditions.push(clause.clause);
    params.push(...clause.params);
  }

  // Add filter conditions
  const filterConditions = buildFilterConditions(options?.filter);
  conditions.push(...filterConditions);

  if (conditions.length > 0) {
    sql += ' WHERE ' + conditions.join(' AND ');
  }

  sql += ' ORDER BY ts ASC, guid ASC';

  if (options?.limit !== undefined) {
    sql += ' LIMIT ?';
    params.push(options.limit);
  }

  const stmt = db.prepare(sql);
  const rows = stmt.all(...params) as MessageRow[];

  return rows.map(parseMessageRow);
}

/**
 * Get messages where the given prefix is mentioned.
 * Uses prefix matching: @alice matches messages mentioning alice.1, alice.419, etc.
 * Also includes @all broadcasts.
 */
export function getMessagesWithMention(
  db: Database.Database,
  mentionPrefix: string,
  options?: MessageQueryOptions
): Message[] {
  const sinceCursor = resolveCursor(db, options?.since);
  if (options?.since && !sinceCursor) {
    throw new Error(`Message not found: ${options.since}`);
  }

  const beforeCursor = resolveCursor(db, options?.before);
  if (options?.before && !beforeCursor) {
    throw new Error(`Message not found: ${options.before}`);
  }

  // Determine if we should filter by read state
  const filterUnread = options?.unreadOnly ?? false;
  const agentPrefix = options?.agentPrefix ?? mentionPrefix;

  // Build the base query that filters for mentions
  let sql = `
    SELECT DISTINCT m.* FROM mm_messages m, json_each(m.mentions) j
  `;

  // Add LEFT JOIN for read receipts if filtering by unread
  if (filterUnread) {
    sql += `
    LEFT JOIN mm_read_receipts r
      ON m.guid = r.message_guid AND r.agent_prefix = ?
    `;
  }

  sql += `
    WHERE (j.value = 'all' OR j.value = ? OR j.value LIKE ?)
  `;

  const params: any[] = [];

  if (filterUnread) {
    params.push(agentPrefix);
  }

  params.push(mentionPrefix, `${mentionPrefix}.%`);

  // Add additional filters
  const conditions: string[] = [];

  // Exclude archived messages by default
  if (options?.includeArchived !== true) {
    conditions.push('m.archived_at IS NULL');
  }

  // Filter for unread messages
  if (filterUnread) {
    conditions.push('r.message_guid IS NULL');
  }

  if (sinceCursor) {
    const clause = buildCursorCondition('m.', '>', sinceCursor);
    conditions.push(clause.clause);
    params.push(...clause.params);
  }

  if (beforeCursor) {
    const clause = buildCursorCondition('m.', '<', beforeCursor);
    conditions.push(clause.clause);
    params.push(...clause.params);
  }

  if (conditions.length > 0) {
    sql += ' AND ' + conditions.join(' AND ');
  }

  sql += ' ORDER BY m.ts ASC, m.guid ASC';

  if (options?.limit !== undefined) {
    sql += ' LIMIT ?';
    params.push(options.limit);
  }

  const stmt = db.prepare(sql);
  const rows = stmt.all(...params) as MessageRow[];

  return rows.map(parseMessageRow);
}

/**
 * Get the last message cursor.
 * Used for --since polling.
 * @returns null if no messages exist
 */
export function getLastMessageCursor(db: Database.Database): MessageCursor | null {
  const stmt = db.prepare(`
    SELECT guid, ts FROM mm_messages
    ORDER BY ts DESC, guid DESC
    LIMIT 1
  `);
  const result = stmt.get() as { guid: string; ts: number } | undefined;

  return result ? { guid: result.guid, ts: result.ts } : null;
}

/**
 * Get a single message by ID.
 * @returns Message or undefined if not found
 */
export function getMessage(db: Database.Database, messageId: string): Message | undefined {
  const stmt = db.prepare('SELECT * FROM mm_messages WHERE guid = ?');
  const row = stmt.get(messageId) as MessageRow | undefined;

  return row ? parseMessageRow(row) : undefined;
}

/**
 * Get a message by GUID prefix (for short IDs like #abc).
 * Returns undefined if no match or multiple matches.
 */
export function getMessageByPrefix(db: Database.Database, prefix: string): Message | undefined {
  // Normalize: remove msg- prefix if present
  let normalizedPrefix = prefix;
  if (normalizedPrefix.toLowerCase().startsWith('msg-')) {
    normalizedPrefix = normalizedPrefix.slice(4);
  }

  const rows = db.prepare(`
    SELECT * FROM mm_messages
    WHERE guid LIKE ?
    ORDER BY ts DESC
    LIMIT 2
  `).all(`msg-${normalizedPrefix}%`) as MessageRow[];

  // Only return if exactly one match
  if (rows.length === 1) {
    return parseMessageRow(rows[0]);
  }

  return undefined;
}

/**
 * Edit a message.
 * Verifies ownership, updates body and edited_at, emits edit event.
 * @throws if message doesn't exist or agent doesn't own it
 */
export function editMessage(
  db: Database.Database,
  messageId: string,
  newBody: string,
  agentId: string
): void {
  const msg = getMessage(db, messageId);
  if (!msg) {
    throw new Error(`Message ${messageId} not found`);
  }

  if (msg.from_agent !== agentId) {
    throw new Error(`Cannot edit message from another agent (message from ${msg.from_agent})`);
  }

  const editedAt = Math.floor(Date.now() / 1000);

  const stmt = db.prepare(
    'UPDATE mm_messages SET body = ?, edited_at = ? WHERE guid = ?'
  );
  stmt.run(newBody, editedAt, messageId);

  const mentions = [agentId];
  createMessage(db, {
    from_agent: 'system',
    body: `update: _@${agentId} edited message #${messageId}_`,
    mentions,
    type: 'event',
  });
}

/**
 * Delete a message by marking it as deleted (archives it).
 */
export function deleteMessage(
  db: Database.Database,
  messageId: string
): void {
  const msg = getMessage(db, messageId);
  if (!msg) {
    throw new Error(`Message ${messageId} not found`);
  }

  const deletedAt = Math.floor(Date.now() / 1000);

  // Mark as deleted by setting body to [deleted] and archiving
  const stmt = db.prepare(
    'UPDATE mm_messages SET body = ?, archived_at = ? WHERE guid = ?'
  );
  stmt.run('[deleted]', deletedAt, messageId);
}

// Config functions

/**
 * Get config value by key.
 * @returns value or undefined if not set
 */
export function getConfig(db: Database.Database, key: string): string | undefined {
  const stmt = db.prepare('SELECT value FROM mm_config WHERE key = ?');
  const result = stmt.get(key) as { value: string } | undefined;

  return result?.value;
}

/**
 * Set config value.
 * Uses INSERT OR REPLACE for upsert.
 */
export function setConfig(db: Database.Database, key: string, value: string): void {
  const stmt = db.prepare('INSERT OR REPLACE INTO mm_config (key, value) VALUES (?, ?)');
  stmt.run(key, value);
}

/**
 * Get all config entries.
 */
export function getAllConfig(db: Database.Database): ConfigEntry[] {
  const stmt = db.prepare('SELECT * FROM mm_config ORDER BY key');
  return stmt.all() as ConfigEntry[];
}

// Linked project functions

/**
 * Get linked project by alias.
 */
export function getLinkedProject(db: Database.Database, alias: string): LinkedProject | undefined {
  const stmt = db.prepare('SELECT * FROM mm_linked_projects WHERE alias = ?');
  return stmt.get(alias) as LinkedProject | undefined;
}

/**
 * Get all linked projects.
 */
export function getLinkedProjects(db: Database.Database): LinkedProject[] {
  const stmt = db.prepare('SELECT * FROM mm_linked_projects ORDER BY alias');
  return stmt.all() as LinkedProject[];
}

/**
 * Link a project (store in database).
 * @param alias - short name for --project flag
 * @param path - absolute path to project root
 * Note: Caller is responsible for validating path exists and contains .beads/
 */
export function linkProject(db: Database.Database, alias: string, path: string): void {
  const stmt = db.prepare('INSERT OR REPLACE INTO mm_linked_projects (alias, path) VALUES (?, ?)');
  stmt.run(alias, path);
}

/**
 * Unlink a project.
 * @returns true if project was removed, false if didn't exist
 */
export function unlinkProject(db: Database.Database, alias: string): boolean {
  const stmt = db.prepare('DELETE FROM mm_linked_projects WHERE alias = ?');
  const result = stmt.run(alias);

  return result.changes > 0;
}

// Filter functions

/**
 * Get filter preferences for an agent.
 * @returns Filter or null if not set
 */
export function getFilter(db: Database.Database, agentId: string): Filter | null {
  const stmt = db.prepare('SELECT * FROM mm_filters WHERE agent_id = ?');
  const row = stmt.get(agentId) as FilterRow | undefined;
  if (!row) return null;
  return row;
}

/**
 * Set filter preferences for an agent.
 * Uses upsert to replace existing filter.
 */
export function setFilter(db: Database.Database, filter: Filter): void {
  const stmt = db.prepare(`
    INSERT INTO mm_filters (agent_id, mentions_pattern)
    VALUES (?, ?)
    ON CONFLICT(agent_id) DO UPDATE SET
      mentions_pattern = excluded.mentions_pattern
  `);
  stmt.run(filter.agent_id, filter.mentions_pattern);
}

/**
 * Clear filter preferences for an agent.
 */
export function clearFilter(db: Database.Database, agentId: string): void {
  const stmt = db.prepare('DELETE FROM mm_filters WHERE agent_id = ?');
  stmt.run(agentId);
}

// Read receipt functions

/**
 * Mark messages as read by an agent.
 * Uses agent prefix (base name) for broader tracking.
 * @param db - Database instance
 * @param messageIds - Message IDs to mark as read
 * @param agentPrefix - Agent base name (e.g., "alice" not "alice.1")
 */
export function markMessagesRead(
  db: Database.Database,
  messageIds: string[],
  agentPrefix: string
): void {
  if (messageIds.length === 0) return;

  const now = Math.floor(Date.now() / 1000);
  const stmt = db.prepare(`
    INSERT OR IGNORE INTO mm_read_receipts (message_guid, agent_prefix, read_at)
    VALUES (?, ?, ?)
  `);

  for (const id of messageIds) {
    stmt.run(id, agentPrefix, now);
  }
}

/**
 * Get all agents who have read a message.
 * @param db - Database instance
 * @param messageId - Message ID
 * @returns Array of agent prefixes sorted by read time
 */
export function getReadReceipts(db: Database.Database, messageId: string): string[] {
  const stmt = db.prepare(
    'SELECT agent_prefix FROM mm_read_receipts WHERE message_guid = ? ORDER BY read_at'
  );
  const rows = stmt.all(messageId) as ReadReceipt[];
  return rows.map(r => r.agent_prefix);
}

/**
 * Get count of agents who have read a message.
 * @param db - Database instance
 * @param messageId - Message ID
 * @returns Number of agents who have read this message
 */
export function getReadReceiptCount(db: Database.Database, messageId: string): number {
  const stmt = db.prepare(
    'SELECT COUNT(*) as count FROM mm_read_receipts WHERE message_guid = ?'
  );
  const result = stmt.get(messageId) as { count: number };
  return result.count;
}

/**
 * Archive messages before a given message cursor.
 * Sets archived_at timestamp for all messages before the cursor.
 * @param db - Database instance
 * @param beforeId - Archive all messages before this cursor or GUID
 * @returns Number of messages archived
 */
export function archiveMessages(db: Database.Database, beforeId?: MessageCursor | string): number {
  const archivedAt = Math.floor(Date.now() / 1000);

  if (!beforeId) {
    const stmt = db.prepare(
      'UPDATE mm_messages SET archived_at = ? WHERE archived_at IS NULL'
    );
    const result = stmt.run(archivedAt);
    return result.changes;
  }

  const cursor = resolveCursor(db, beforeId);
  if (!cursor) {
    throw new Error(`Message not found: ${beforeId}`);
  }
  const clause = buildCursorCondition('', '<', cursor);
  const stmt = db.prepare(
    `UPDATE mm_messages SET archived_at = ? WHERE ${clause.clause} AND archived_at IS NULL`
  );
  const result = stmt.run(archivedAt, ...clause.params);
  return result.changes;
}

/**
 * Get all replies to a message (thread).
 * Returns the parent message plus all direct replies, in chronological order.
 * @param db - Database instance
 * @param messageId - Parent message GUID
 * @returns Array of messages in thread (parent first, then replies)
 */
export function getThread(db: Database.Database, messageId: string): Message[] {
  const stmt = db.prepare(`
    SELECT * FROM mm_messages
    WHERE guid = ? OR reply_to = ?
    ORDER BY CASE WHEN guid = ? THEN 0 ELSE 1 END, ts ASC, guid ASC
  `);
  const rows = stmt.all(messageId, messageId, messageId) as MessageRow[];
  return rows.map(parseMessageRow);
}

/**
 * Get reply count for a message.
 * @param db - Database instance
 * @param messageId - Parent message GUID
 * @returns Number of direct replies
 */
export function getReplyCount(db: Database.Database, messageId: string): number {
  const stmt = db.prepare('SELECT COUNT(*) as count FROM mm_messages WHERE reply_to = ?');
  const result = stmt.get(messageId) as { count: number };
  return result.count;
}

// Claim functions

/**
 * Prune expired claims from the database.
 * @param db - Database instance
 * @returns Number of claims pruned
 */
export function pruneExpiredClaims(db: Database.Database): number {
  const now = Math.floor(Date.now() / 1000);
  const stmt = db.prepare('DELETE FROM mm_claims WHERE expires_at IS NOT NULL AND expires_at < ?');
  const result = stmt.run(now);
  return result.changes;
}

/**
 * Create a new claim.
 * @param db - Database instance
 * @param claim - Claim data
 * @returns Created claim with id
 * @throws if claim already exists (UNIQUE constraint on claim_type, pattern)
 */
export function createClaim(db: Database.Database, claim: ClaimInput): Claim {
  const now = Math.floor(Date.now() / 1000);
  const stmt = db.prepare(`
    INSERT INTO mm_claims (agent_id, claim_type, pattern, reason, created_at, expires_at)
    VALUES (?, ?, ?, ?, ?, ?)
  `);

  try {
    const result = stmt.run(
      claim.agent_id,
      claim.claim_type,
      claim.pattern,
      claim.reason ?? null,
      now,
      claim.expires_at ?? null
    );

    return {
      id: Number(result.lastInsertRowid),
      agent_id: claim.agent_id,
      claim_type: claim.claim_type,
      pattern: claim.pattern,
      reason: claim.reason ?? null,
      created_at: now,
      expires_at: claim.expires_at ?? null,
    };
  } catch (error: any) {
    // Check if it's a UNIQUE constraint violation
    if (error.code === 'SQLITE_CONSTRAINT_UNIQUE') {
      const existing = getClaim(db, claim.claim_type, claim.pattern);
      if (existing) {
        throw new Error(`Already claimed by @${existing.agent_id}: ${claim.claim_type}:${claim.pattern}`);
      }
    }
    throw error;
  }
}

/**
 * Get a claim by type and pattern.
 * @param db - Database instance
 * @param claimType - Claim type
 * @param pattern - Pattern to match
 * @returns Claim or undefined
 */
export function getClaim(db: Database.Database, claimType: ClaimType, pattern: string): Claim | undefined {
  const stmt = db.prepare('SELECT * FROM mm_claims WHERE claim_type = ? AND pattern = ?');
  return stmt.get(claimType, pattern) as Claim | undefined;
}

/**
 * Get all claims for an agent.
 * @param db - Database instance
 * @param agentId - Agent ID
 * @returns Array of claims
 */
export function getClaimsByAgent(db: Database.Database, agentId: string): Claim[] {
  const stmt = db.prepare('SELECT * FROM mm_claims WHERE agent_id = ? ORDER BY created_at');
  return stmt.all(agentId) as Claim[];
}

/**
 * Get all claims of a specific type.
 * @param db - Database instance
 * @param claimType - Claim type
 * @returns Array of claims
 */
export function getClaimsByType(db: Database.Database, claimType: ClaimType): Claim[] {
  const stmt = db.prepare('SELECT * FROM mm_claims WHERE claim_type = ? ORDER BY created_at');
  return stmt.all(claimType) as Claim[];
}

/**
 * Get all active (non-expired) claims.
 * @param db - Database instance
 * @returns Array of claims
 */
export function getAllClaims(db: Database.Database): Claim[] {
  // First prune expired claims
  pruneExpiredClaims(db);

  const stmt = db.prepare('SELECT * FROM mm_claims ORDER BY created_at');
  return stmt.all() as Claim[];
}

/**
 * Delete a specific claim.
 * @param db - Database instance
 * @param claimType - Claim type
 * @param pattern - Pattern to match
 * @returns true if claim was deleted
 */
export function deleteClaim(db: Database.Database, claimType: ClaimType, pattern: string): boolean {
  const stmt = db.prepare('DELETE FROM mm_claims WHERE claim_type = ? AND pattern = ?');
  const result = stmt.run(claimType, pattern);
  return result.changes > 0;
}

/**
 * Delete all claims for an agent.
 * @param db - Database instance
 * @param agentId - Agent ID
 * @returns Number of claims deleted
 */
export function deleteClaimsByAgent(db: Database.Database, agentId: string): number {
  const stmt = db.prepare('DELETE FROM mm_claims WHERE agent_id = ?');
  const result = stmt.run(agentId);
  return result.changes;
}

/**
 * Find file claims that conflict with given file paths.
 * Uses micromatch for glob pattern matching.
 * @param db - Database instance
 * @param filePaths - File paths to check
 * @param excludeAgent - Agent to exclude from conflict check (optional)
 * @returns Array of conflicting claims
 */
export function findConflictingFileClaims(
  db: Database.Database,
  filePaths: string[],
  excludeAgent?: string
): Claim[] {
  // Get all file claims
  const allFileClaims = getClaimsByType(db, 'file');

  // Filter out claims from the excluded agent
  const otherClaims = excludeAgent
    ? allFileClaims.filter(c => c.agent_id !== excludeAgent)
    : allFileClaims;

  // Find claims that conflict with any of the file paths
  const conflicts: Claim[] = [];

  for (const claim of otherClaims) {
    // Check if the claim pattern matches any of the file paths
    const matchingFiles = micromatch(filePaths, claim.pattern);
    if (matchingFiles.length > 0) {
      conflicts.push(claim);
    }
  }

  return conflicts;
}

/**
 * Check if a file path conflicts with any existing claims (excluding an agent).
 * @param db - Database instance
 * @param filePath - Single file path to check
 * @param excludeAgent - Agent to exclude from conflict check
 * @returns Conflicting claim or undefined
 */
export function checkFileConflict(
  db: Database.Database,
  filePath: string,
  excludeAgent?: string
): Claim | undefined {
  const conflicts = findConflictingFileClaims(db, [filePath], excludeAgent);
  return conflicts[0];
}

/**
 * Update claims when renaming an agent.
 * @param db - Database instance
 * @param oldId - Old agent ID
 * @param newId - New agent ID
 */
export function updateClaimsAgentId(db: Database.Database, oldId: string, newId: string): void {
  const stmt = db.prepare('UPDATE mm_claims SET agent_id = ? WHERE agent_id = ?');
  stmt.run(newId, oldId);
}

/**
 * Get claim counts grouped by agent.
 * @param db - Database instance
 * @returns Map of agent_id to claim count
 */
export function getClaimCountsByAgent(db: Database.Database): Map<string, number> {
  // First prune expired claims
  pruneExpiredClaims(db);

  const stmt = db.prepare(`
    SELECT agent_id, COUNT(*) as count
    FROM mm_claims
    GROUP BY agent_id
  `);
  const rows = stmt.all() as { agent_id: string; count: number }[];

  const counts = new Map<string, number>();
  for (const row of rows) {
    counts.set(row.agent_id, row.count);
  }
  return counts;
}
