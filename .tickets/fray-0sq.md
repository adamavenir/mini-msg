---
id: fray-0sq
status: closed
deps: []
links: []
created: 2025-12-05T15:50:18.291387-08:00
type: task
priority: 2
---
# Create chat core logic

Create the core chat session logic for polling and sending messages.

## New File: src/chat/core.ts

```typescript
export interface ChatSession {
  username: string;
  projectName: string;
  db: Database.Database;
  lastMessageId: number;
}

export function createSession(opts: {
  db: Database.Database;
  projectName: string;
  username: string;
  lastMessageId?: number;
}): ChatSession;

export function pollNewMessages(session: ChatSession): Message[];

export function sendUserMessage(session: ChatSession, body: string): Message;
```

## Behavior

### pollNewMessages
- Query messages with `since: session.lastMessageId`
- Update `session.lastMessageId` to latest
- Return array of new messages

### sendUserMessage
- Extract mentions from body
- Create message with `type: 'user'` and `from_agent: session.username`
- Return created message for immediate display

## Exit Criteria
- Session tracks state correctly
- Polling returns only new messages
- User messages have correct type and sender
- Unit tests for core functions


