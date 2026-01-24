---
id: fray-1mk
status: closed
deps: [fray-qqv]
links: []
created: 2025-12-19T11:57:25.36173-08:00
type: task
priority: 1
---
# P1.1: JSON output for commands

Add --json flag to commands for programmatic access.

Commands to update:
1. mm here --json
   Output: {"agents": [{"agent_id": "usr-xxx", "display_name": "alice", "goal": "...", "last_active": "ISO", "message_count": N, "claim_count": N}], "total": N}

2. mm history <agent> --json
   Output: {"agent": "alice", "agent_id": "usr-xxx", "messages": [{"id": "msg-xxx", "agent_id": "usr-xxx", "body": "...", "created_at": "ISO", "age_seconds": N, "mentions": [], "reply_to": null}], "total": N}

3. mm between <a> <b> --json
   Output: {"agents": ["alice", "bob"], "messages": [...], "total": N}

Implementation:
- Add --json flag parsing to each command
- Return structured JSON (pretty-printed)
- Include full GUIDs (no display IDs)
- Use ISO 8601 timestamps

References: PLAN.md section 9
Critical files: src/commands/here.ts, src/commands/history.ts (NEW), src/commands/between.ts (NEW)


