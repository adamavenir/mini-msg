---
id: fray-ql6
status: closed
deps: [fray-3vp, fray-z3l]
links: []
created: 2025-12-19T13:26:44.503921-08:00
type: task
priority: 0
---
# P0.0: Minimal GUID proof-of-concept

Create minimal vertical slice to prove GUID architecture before full implementation.

Goal: Simple working example of GUID→JSONL→SQLite flow

Implement:
1. GUID generation function (8-char base58)
2. Single JSONL writer (messages.jsonl only)
3. Simple SQLite rebuild from JSONL
4. Basic .mm/mm-config.json with channel_id

Test with:
- mm post creates msg-GUID
- Writes to messages.jsonl
- Rebuilds SQLite on next read
- Verify GUID stable across rebuilds

Files:
- src/core/guid.ts (NEW) - generateGuid(type)
- src/db/jsonl.ts (NEW) - appendMessage(), readMessages()
- src/db/schema.ts - Add guid column to mm_messages
- src/commands/post.ts - Generate GUID, write JSONL

Acceptance:
- Can post message
- JSONL file created with valid format
- SQLite rebuilt from JSONL
- GUIDs preserved

This proves the architecture works before implementing all the features.

References: PLAN.md sections 1, 2
Critical files: src/core/guid.ts (NEW), src/db/jsonl.ts (NEW)


