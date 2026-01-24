---
id: fray-5ar
status: closed
deps: []
links: []
created: 2025-12-05T15:50:35.218142-08:00
type: task
priority: 2
---
# Implement bdm chat command

Implement the `bdm chat` command that ties together display, input, and core logic.

## New File: src/commands/chat.ts

```typescript
export function chatCommand(): Command {
  return new Command('chat')
    .description('Interactive chat mode')
    .option('--last <n>', 'show last N messages', '20')
    .action((options, cmd) => {
      // ... implementation
    });
}
```

## Behavior

1. **Startup**
   - Get context (db, project)
   - Read username from config (error if not set)
   - Show last N messages
   - Start polling interval (1 second)
   - Start input handling

2. **Message Display**
   - New messages from poll rendered via display
   - User's own messages rendered immediately after send

3. **User Input**
   - On line submit: send message, render it
   - On quit (Ctrl+C): cleanup and exit

4. **Cleanup**
   - Clear interval
   - Destroy display/input
   - Close database

## Wire Up

In src/cli.ts:
- Import chatCommand
- Add to program
- Add 'chat' to knownCommands

## Error Handling
- "Set username first: bdm config username <name>" if no username
- "--json not supported for interactive chat" if --json flag

## Exit Criteria
- `bdm chat` starts interactive mode
- Messages from other agents appear in real-time
- User can type and send messages
- Ctrl+C exits cleanly


