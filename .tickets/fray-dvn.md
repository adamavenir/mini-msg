---
id: fray-dvn
status: closed
deps: [fray-3vp]
links: []
created: 2025-12-19T11:57:09.444343-08:00
type: task
priority: 0
---
# P0.3: SQLite rebuild from JSONL

Implement SQLite rebuild from JSONL on startup (beads-style staleness detection).

Implementation:
- Check JSONL mtime vs SQLite mtime on db connection
- If JSONL newer or SQLite missing: rebuild from JSONL
- Read messages.jsonl, agents.jsonl, .mm/mm-config.json
- Drop and recreate tables, insert all records
- Update .mm/.gitignore to ignore *.db, *.db-wal, *.db-shm

Staleness check:
- Compare max(messages.jsonl mtime, agents.jsonl mtime) vs mm.db mtime
- Log: "Rebuilding SQLite from JSONL (stale database)"

References: PLAN.md section 2, beads staleness detection
Critical files: src/db/schema.ts, src/db/jsonl.ts, .mm/.gitignore


