---
id: fray-6qu
status: closed
deps: [fray-qqv]
links: []
created: 2025-12-19T11:57:10.559979-08:00
type: task
priority: 0
---
# P0.7: Migration from current format

Implement mm migrate command to convert v0.1.0 to v0.2.0 GUID format.

Migration steps:
1. Check if .mm/ exists and is old format (no .mm/mm-config.json)
2. Backup: cp -r .mm/ .mm.bak/
3. Generate GUIDs for all existing messages and agents
4. Create .mm/mm-config.json with channel_id
5. Create messages.jsonl from SQLite messages
6. Create agents.jsonl from SQLite agents
7. Rebuild SQLite with new schema
8. Success message: "Migration complete. Backup at .mm.bak/"

Old schema detection:
- Check for absence of .mm/mm-config.json
- Check for presence of .mm/mm.db

GUID generation:
- Messages: msg-<8chars> (preserve order by created_at)
- Agents: usr-<8chars>
- Channel: ch-<8chars> (single channel, prompt for name)

References: PLAN.md section Migration Path
Critical files: src/commands/migrate.ts, src/db/schema.ts, src/db/jsonl.ts


