---
id: fray-qqv
status: closed
deps: [fray-z3l]
links: []
created: 2025-12-19T11:57:10.004318-08:00
type: task
priority: 0
---
# P0.5: Agent GUID and global registry

Implement agent GUID system with progressive discovery.

Agent storage:
- known_agents in .mm/mm-config.json (source of truth, committed)
- Global config (~/.config/mm/) tracks channels only (NOT agents)

mm new <name> flow:
1. Check .mm/mm-config.json known_agents
2. If exists: Prompt "Use existing @<name> (<usr-guid>)? [Y/n]"
   - Y: Reuse GUID
   - n: Generate new usr-<guid>
3. If not exists: Generate new usr-<guid>
4. Add to known_agents in .mm/mm-config.json
5. Append to agents.jsonl

Agent fields:
- id: usr-<8chars> (GUID)
- name: Simple name (e.g., "devrel")
- global_name: Prefixed name (e.g., "mm-devrel")
- home_channel: ch-<guid> or null for global agents
- created_at: ISO timestamp
- status: "active" or "inactive"

Auto-nick generation:
- If home_channel == current channel: nick = name
- Else: nick = global_name

References: PLAN.md sections 6, 3
Critical files: src/commands/new.ts, src/core/config.ts, src/db/jsonl.ts


