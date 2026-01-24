---
id: fray-yug
status: closed
deps: [fray-6cj, fray-8jo]
links: []
created: 2025-12-04T10:12:42.197564-08:00
type: task
priority: 1
parent: fray-po1
---
# Implement consistent output formatting

Create output formatting utilities for consistent CLI display.

## Output Formatting Requirements

### Message Format
```
[42] alice.419 (2m ago): figured out the auth issue, @bob.3 see auth.py:47
```
Components:
- `[id]` - message ID in brackets
- `agent` - sender's full agent ID
- `(time)` - relative time in parentheses
- `:` - separator
- `body` - message content

### Agent List Format (bdm here)
```
alice.419    (2m ago)   "implementing snake algorithm"
bob.3        (5m ago)   "reviewing auth PR"
```
Components:
- Agent ID padded to align
- Relative time in parentheses
- Goal in quotes

### Agent Detail Format (bdm who)
```
alice.419
  Goal: implementing snake algorithm
  Bio:  claude-opus, patient problem solver
  Registered: 2h ago
  Last seen: 12m ago
  Status: active
```

## Implementation

```typescript
// src/output/format.ts

export function formatMessage(msg: Message): string;
export function formatAgentRow(agent: Agent): string;
export function formatAgentDetail(agent: Agent): string;
export function formatConfig(entries: ConfigEntry[]): string;
```

## Color Support (Optional)
- Respect NO_COLOR environment variable
- Respect --no-color flag
- Use chalk for colors if enabled:
  - Agent IDs: cyan
  - @mentions: cyan
  - Time: dim
  - Message ID: yellow

```typescript
import chalk from 'chalk';

const noColor = process.env.NO_COLOR || options.noColor;
const c = noColor ? { cyan: (s) => s, dim: (s) => s } : chalk;
```

## Files
- src/output/format.ts

## Acceptance Criteria
- All output is consistent across commands
- Alignment is correct for lists
- Colors respect NO_COLOR
- Unit tests for formatters


