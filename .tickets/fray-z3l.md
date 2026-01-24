---
id: fray-z3l
status: closed
deps: [fray-7kr]
links: []
created: 2025-12-19T11:57:09.724922-08:00
type: task
priority: 0
---
# P0.4: Channel GUID system

Implement channel GUID system with project-level config.

Project config (.mm/mm-config.json, COMMITTED to git):
{
  "version": 1,
  "channel_id": "ch-mmdev12",
  "channel_name": "mm",
  "created_at": "2025-12-19T10:00:00Z",
  "known_agents": {}
}

Global config (~/.config/mm/mm-config.json, per-machine):
{
  "version": 1,
  "channels": {
    "ch-mmdev12": {"name": "mm", "path": "/Users/adam/dev/mini-msg"}
  },
  "current_channel": "ch-mmdev12"
}

Implementation:
- Add channel_id TEXT column to mm_messages table
- Create .mm/mm-config.json on mm init (prompt for channel name, default: dirname)
- Generate channel GUID (ch-<8chars>)
- Register in ~/.config/mm/mm-config.json
- Create src/core/config.ts for config file handling

mm init flow:
1. Prompt: "Channel name? [dirname]:"
2. Generate ch-<guid>
3. Write .mm/mm-config.json
4. Register in ~/.config/mm/mm-config.json

References: PLAN.md sections 3, 4
Critical files: src/commands/init.ts, src/core/config.ts (NEW), src/db/schema.ts


