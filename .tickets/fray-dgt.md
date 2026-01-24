---
id: fray-dgt
status: closed
deps: [fray-6cj, fray-1wj]
links: []
created: 2025-12-04T09:59:53.467278-08:00
type: task
priority: 1
parent: fray-08v
---
# Implement message query functions

Create query functions for message CRUD operations in src/db/queries.ts.

## Functions to Implement

```typescript
// src/db/queries.ts (message functions)

import type { Message, MessageRow, MessageQueryOptions } from '../types.js';
import type Database from 'better-sqlite3';

/**
 * Create a new message.
 * @param mentions - Pre-extracted mentions array (caller extracts from body)
 * @returns The created message with ID
 */
export function createMessage(
  db: Database, 
  message: { from_agent: string; body: string; mentions: string[] }
): Message;

/**
 * Get messages with optional filtering.
 * Returns in chronological order (oldest first).
 */
export function getMessages(
  db: Database, 
  options?: MessageQueryOptions
): Message[];

/**
 * Get messages where the given prefix is mentioned.
 * Uses prefix matching: @alice matches messages mentioning alice.1, alice.419, etc.
 * Also includes @all broadcasts.
 */
export function getMessagesWithMention(
  db: Database, 
  mentionPrefix: string, 
  options?: MessageQueryOptions
): Message[];

/**
 * Get the last message ID.
 * Used for --since polling.
 * @returns 0 if no messages exist
 */
export function getLastMessageId(db: Database): number;

/**
 * Convert MessageRow to Message (parse mentions JSON).
 */
export function parseMessageRow(row: MessageRow): Message;
```

## Implementation Notes
- mentions are stored as JSON array in database
- **Important**: Caller is responsible for extracting mentions from body before calling createMessage. The extractMentions() function is implemented later in Phase 3 (bdm-92p). For now, just accept mentions as a parameter.
- Prefix matching for mentions: check if any mention starts with prefix
- @all is stored literally and matched specially

## Query for Mention Matching
```sql
-- For each message, check if any mention in the JSON array starts with prefix
-- SQLite approach: use json_each to expand array
SELECT * FROM bdm_messages 
WHERE id IN (
  SELECT m.id FROM bdm_messages m, json_each(m.mentions) j
  WHERE j.value = 'all' OR j.value LIKE ? || '%'
)
ORDER BY id
LIMIT ?
```

## Files
- src/db/queries.ts (add message functions)

## Acceptance Criteria
- Messages are created with provided mentions array
- Mention filtering uses prefix matching
- @all broadcasts are always included
- Pagination via limit/since works correctly
- Unit tests for all functions


