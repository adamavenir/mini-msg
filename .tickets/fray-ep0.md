---
id: fray-ep0
status: closed
deps: []
links: []
created: 2025-12-09T06:12:02.890916-08:00
type: bug
priority: 0
---
# Silence system messages by default

System messages (beads issue events) are flooding chat and causing noise.

## Problem

Running `bdm watch` triggers a flood of system messages about issue status changes, which then appear in `bdm chat`. These messages clutter the conversation and make it hard to follow agent-to-agent communication.

Example spam:
```
[#party #party-3f5]  @system:
update: _party-3f5 status_changed_
```

## Solution

**Default behavior**: Silence system messages in all modes (get, chat, watch, @mentions)

**Opt-in**: Add `--show-updates` flag to display system messages when explicitly requested

```bash
bdm chat --show-updates      # Show system messages
bdm watch --show-updates     # Include events
bdm get alice.1 --show-updates  # Show events in combined view
```

## Implementation

- Add `exclude_events` parameter to message queries (default: true)
- Update all commands to respect this flag
- Add `--show-updates` option to: chat, watch, get, @mentions commands

## Follow-up

Create a separate card to revisit event notification design - we may want smarter filtering (e.g., only show events for issues mentioned in conversation).

## Files

- src/db/queries.ts - Add exclude_events to getMessages query
- src/commands/chat.ts - Add --show-updates flag
- src/commands/watch.ts - Add --show-updates flag, default exclude events
- src/commands/get.ts - Add --show-updates flag
- src/commands/mentions.ts - Add --show-updates flag


