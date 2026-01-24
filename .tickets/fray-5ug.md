---
id: fray-5ug
status: closed
deps: []
links: []
created: 2025-12-09T06:12:03.117969-08:00
type: feature
priority: 1
---
# Add message archive and prune command

Add ability to archive old messages to keep active message list manageable.

## Commands

**Prune current messages:**
```bash
bdm prune              # Archive all messages before current (max message ID)
bdm prune --since 100  # Archive all messages before message 100
```

## Behavior

- Archived messages remain in database but are excluded from default queries
- Add `archived_at` column to `bdm_messages` table (nullable timestamp)
- Default queries filter WHERE `archived_at IS NULL`
- Archived messages retrievable with explicit flag: `bdm get --archived`

## Schema Change

```sql
ALTER TABLE bdm_messages ADD COLUMN archived_at INTEGER;
CREATE INDEX idx_bdm_messages_archived ON bdm_messages(archived_at);
```

## Query Updates

All message queries should filter out archived messages by default:
- getMessages(): Add WHERE archived_at IS NULL
- getMessagesWithMention(): Add WHERE archived_at IS NULL
- Add --archived flag to include archived messages

## Implementation

- src/db/schema.ts - Add migration for archived_at column
- src/db/queries.ts - Update queries to filter archived, add archiveMessages() function
- src/commands/prune.ts - New command to archive messages
- src/commands/get.ts, mentions.ts, chat.ts, watch.ts - Add --archived flag


