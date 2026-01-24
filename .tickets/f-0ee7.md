---
id: f-0ee7
status: closed
deps: [f-366d, f-82a4]
links: []
created: 2026-01-24T05:06:34Z
type: task
priority: 1
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase3]
---
# Phase 3: Agent descriptor events

Implement agent_descriptor events for remote agent discovery.

**Context**: Read docs/MULTI-MACHINE-SYNC-PLAN.md section 3.2.

**Files to modify**: 
- internal/db/jsonl_append.go (emit descriptor)
- internal/db/jsonl_rebuild.go (apply descriptors)

**Event location**: shared/machines/{local}/agent-state.jsonl

**Event format**:
```jsonl
{"type":"agent_descriptor","agent_id":"opus","display_name":"Opus","capabilities":["code","review"],"seq":1,"ts":...}
```

**Creation triggers**:
1. On first message from agent on this machine (check if descriptor exists first)
2. Migration scans messages.jsonl for unique from_agent, emits descriptors

**Schema**:
- agent_id: required
- display_name: optional (for UI)
- capabilities: optional array (for filtering/display)

**Rebuild applies descriptors**:
- Build agent info map from descriptors
- Use for agent selection UI in join flow
- Overlay with local invoke config

**Tests required**:
- Test descriptor is emitted on first message
- Test descriptor is not duplicated
- Test migration emits descriptors for existing agents
- Test rebuild populates agent info from descriptors

## Acceptance Criteria

- Agent descriptor events implemented
- Emitted on first message from agent
- Migration emits for existing agents
- Rebuild applies descriptors
- Unit tests pass
- go test ./... passes
- Changes committed

