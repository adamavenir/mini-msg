---
id: fray-ebf
status: closed
deps: []
links: []
created: 2025-12-09T06:14:53.848443-08:00
type: bug
priority: 0
---
# Fix bdm watch --last 0 showing all history

Running bdm watch --last 0 should skip history and only stream new messages, but it shows all history first.

## Bug

When running bdm watch --last 0:
1. Shows 0 messages from history (correct)
2. Sets lastMessageId = 0 (because recent.length === 0)
3. Starts polling with since: 0
4. Queries since: 0 returns ALL messages in database
5. Floods output with entire history

## Expected Behavior

bdm watch --last 0 should:
1. Skip showing any history
2. Set lastMessageId to current max message ID
3. Only show NEW messages that arrive after the command starts

## Fix

In src/commands/watch.ts around line 26-45:

```typescript
// Get last message ID - if --last 0, skip to current
let lastMessageId = 0;
const lastN = parseInt(options.last, 10);

if (lastN === 0) {
  // Skip history, start from current
  const maxId = getLastMessageId(db);
  lastMessageId = maxId;
} else {
  // Show recent context first
  const recent = getMessages(db, { limit: lastN });
  if (recent.length > 0) {
    // ... display messages ...
    lastMessageId = recent[recent.length - 1].id;
  }
}
```

## Files

- src/commands/watch.ts - Fix initialization of lastMessageId when --last 0


