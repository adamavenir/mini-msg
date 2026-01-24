---
id: fray-az3
status: closed
deps: []
links: []
created: 2025-12-05T15:49:59.514678-08:00
type: task
priority: 2
---
# Add type column to bdm_messages table

Add a `type` column to distinguish user messages from agent messages.

## Changes

1. **src/db/schema.ts** - Add migration logic:
   - Check if column exists with `PRAGMA table_info(bdm_messages)`
   - If missing: `ALTER TABLE bdm_messages ADD COLUMN type TEXT DEFAULT 'agent'`

2. **src/types.ts** - Update interfaces:
   ```typescript
   export type MessageType = 'user' | 'agent';
   export interface Message {
     // ... existing
     type: MessageType;
   }
   ```

3. **src/db/queries.ts** - Update functions:
   - `createMessage()` accepts optional `type` parameter (default 'agent')
   - `parseMessageRow()` includes type field

## Exit Criteria
- Existing messages have type='agent' after migration
- New messages can specify type='user' or type='agent'
- All existing tests pass


