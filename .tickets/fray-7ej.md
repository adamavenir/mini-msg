---
id: fray-7ej
status: closed
deps: [fray-2vy, fray-xpd, fray-8jo, fray-6u7]
links: []
created: 2025-12-04T10:08:39.507705-08:00
type: task
priority: 1
parent: fray-yh4
---
# Implement presence commands (here, who)

Create presence commands for viewing active agents.

## Commands

### bdm here
List active agents (filters out stale and left agents).

```bash
bdm here                         # active agents only
bdm here --all                   # include stale
```

**Output format:**
```
alice.419    (2m ago)   "implementing snake algorithm"
pm.5         (30s ago)  "overseeing sprint"
bob.3        (5m ago)   "reviewing auth PR"
```

**Implementation:**
1. Get stale_hours from config (default: 4)
2. Query getActiveAgents(db, staleHours) or getAllAgents(db) with --all
3. Filter: left_at IS NULL (unless --all)
4. Sort by last_seen (most recent first)
5. Format output with relative time and goal

**JSON output (--json):**
```json
[
  {"agent_id": "alice.419", "last_seen": 1234567890, "goal": "..."}
]
```

### bdm who <agent>
Show detailed info for agent(s) matching prefix.

```bash
bdm who alice                    # all alice.* agents
bdm who alice.419                # specific agent
bdm who here                     # all active with details
```

**Output format:**
```
alice.419
  Goal: implementing snake algorithm
  Bio:  claude-opus, patient problem solver
  Registered: 2h ago
  Last seen: 12m ago
  Status: active
```

**Implementation:**
1. If "here", get active agents
2. Otherwise, getAgentsByPrefix(db, prefix)
3. Format each agent with full details
4. Status: "active", "stale", or "left"

## Files
- src/commands/here.ts
- src/commands/who.ts

## Acceptance Criteria
- here shows only active agents by default
- here --all includes stale agents
- who shows detailed agent info
- Prefix matching works correctly
- JSON output is valid and complete


