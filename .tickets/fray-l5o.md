---
id: fray-l5o
status: closed
deps: [fray-2vy, fray-dgt, fray-8jo]
links: []
created: 2025-12-04T10:08:56.250496-08:00
type: task
priority: 1
parent: fray-yh4
---
# Implement message display command (default action)

Implement the default bdm command to show room message history.

## Usage

```bash
bdm                              # last 20 messages
bdm --last 50                    # last 50 messages
bdm --since 42                   # messages after ID 42
bdm --all                        # full history
```

## Output Format

```
[42] alice.419 (2m ago): figured out the auth issue, @bob.3 see auth.py:47
[43] bob.3 (1m ago): @alice.419 got it, thanks
[44] pm.5 (30s ago): @all standup in 5
```

Format: `[id] agent (time): body`

## Implementation

1. Parse options (--last, --since, --all)
2. Build MessageQueryOptions from flags
3. Call getMessages(db, options)
4. Format each message:
   - `[{id}] {from_agent} ({relative_time}): {body}`
   - Optionally highlight @mentions in body

## Default Behavior
- No flags: last 20 messages
- --last N: last N messages
- --since ID: messages with id > ID
- --all: no limit (may be large)

## JSON Output (--json)

```json
[
  {
    "id": 42,
    "ts": 1234567890,
    "from_agent": "alice.419",
    "body": "...",
    "mentions": ["bob.3"]
  }
]
```

## Files
- src/commands/messages.ts (or inline in cli.ts default action)

## Acceptance Criteria
- Default shows last 20 messages
- --last, --since, --all work correctly
- Output format matches spec
- Empty room shows appropriate message
- JSON output is valid


