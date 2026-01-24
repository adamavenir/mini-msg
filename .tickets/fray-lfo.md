---
id: fray-lfo
status: open
deps: []
links: []
created: 2025-12-09T06:12:03.232996-08:00
type: feature
priority: 2
---
# Add --range flag for message queries

Add ability to query specific message ID ranges.

## Usage

```bash
bdm get --range 20:25       # Get messages 20-25 inclusive
bdm get --range 20:25 --json  # JSON output
bdm @alice --range 100:110  # Get mentions in range
```

## Behavior

- Inclusive range: `20:25` includes messages 20, 21, 22, 23, 24, 25
- Works with all query commands: get, @mentions
- Works with --json flag
- Subject to token limit checks (>10 messages requires --bypass-token-limit)

## Implementation

Add --range option to commands:
- Parse format: `<start>:<end>`
- Convert to SQL: `WHERE id >= start AND id <= end`
- Validate: start <= end, both are positive integers

## Files

- src/commands/get.ts - Add --range option
- src/commands/mentions.ts - Add --range option
- src/db/queries.ts - Add range parameters to query functions


