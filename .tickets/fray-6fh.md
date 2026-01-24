---
id: fray-6fh
status: closed
deps: []
links: []
created: 2025-12-05T15:50:09.859942-08:00
type: task
priority: 2
---
# Create chat display/input abstractions

Create abstraction interfaces for chat display and input, with simple ANSI/readline implementations.

## New Files

### src/chat/types.ts
```typescript
export interface FormattedMessage {
  id: number;
  projectName: string;
  type: 'user' | 'agent';
  sender: string;
  body: string;
}

export interface ChatDisplay {
  renderMessage(msg: FormattedMessage): void;
  showStatus(text: string): void;
  destroy(): void;
}

export interface ChatInput {
  start(): void;
  onMessage(callback: (text: string) => void): void;
  onQuit(callback: () => void): void;
  destroy(): void;
}
```

### src/chat/display.ts
- ANSI color implementation
- Format: `[#projectname LN] type @name: body`
- Green for users, blue for agents, cyan for @mentions
- Respect NO_COLOR environment variable

### src/chat/input.ts
- Use `readline.createInterface`
- Handle line events for message submission
- Handle close/SIGINT for quit

## Design Notes
- Interfaces allow swapping implementations later (e.g., to ink or blessed)
- Keep implementations minimal for v1

## Exit Criteria
- Interfaces defined in types.ts
- Display prints colored messages to stdout
- Input captures lines and signals quit on Ctrl+C


