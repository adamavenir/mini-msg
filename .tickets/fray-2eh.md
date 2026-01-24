---
id: fray-2eh
status: closed
deps: [fray-l5o, fray-dgt, fray-8jo]
links: []
created: 2025-12-04T10:09:49.263646-08:00
type: task
priority: 1
parent: fray-yh4
---
# Implement watch command (--watch flag)

Implement tail -f style message watching.

## Usage

```bash
bdm --watch                      # tail -f all messages
```

## Implementation

1. Get current last message ID
2. Display existing messages (last N or all)
3. Enter poll loop:
   - Sleep for interval (1 second default)
   - Query messages with --since lastId
   - Display new messages
   - Update lastId
   - Repeat until interrupted (Ctrl+C)

## Poll Loop

```typescript
async function watch(db: Database, startId: number, interval: number = 1000) {
  let lastId = startId;
  
  // Handle Ctrl+C gracefully
  process.on('SIGINT', () => {
    console.log('\nStopped watching.');
    process.exit(0);
  });
  
  while (true) {
    await sleep(interval);
    const newMessages = getMessages(db, { since: lastId });
    
    for (const msg of newMessages) {
      console.log(formatMessage(msg));
      lastId = msg.id;
    }
  }
}
```

## Display Considerations
- Print each new message as it arrives
- No buffering - immediate output
- Clear indication when starting ("Watching for new messages...")
- Clean exit on Ctrl+C

## Optional Enhancements (defer if complex)
- `--interval <ms>` to change poll frequency
- Filter by @mentions while watching
- Bell/notification on new messages

## Files
- src/commands/watch.ts

## Acceptance Criteria
- Displays new messages as they arrive
- Polls at reasonable interval (1s default)
- Handles Ctrl+C gracefully
- Works correctly with concurrent posters


