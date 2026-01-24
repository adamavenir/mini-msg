---
id: f-e5b2
status: closed
deps: [f-ee9a, f-2312]
links: []
created: 2026-01-24T05:06:10Z
type: task
priority: 1
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase3]
---
# Phase 3: Join flow for existing multi-machine projects

Implement the join flow when fray init detects an existing shared/ directory.

**Context**: Read docs/MULTI-MACHINE-SYNC-PLAN.md section 3.1 and SPEC Initialization Flow.

**Files to modify**: internal/command/init.go

**Detection**: fray init detects existing shared/ directory synced from other machines.

**Join flow**:
1. Detect shared/ exists
2. Check machine ID collision (from Phase 2 ticket)
3. Show agent selection UI with capabilities from descriptors
4. Create local/ directory
5. Write machine-id file
6. Register selected agents locally
7. Rebuild database

**Agent selection UI**:
```
Which agents do you want to run on "desktop"?

  [x] opus      (last active: laptop, 2h ago)  [code, review]
  [x] designer  (last active: laptop, 1d ago)  [design]
  [ ] reviewer  (last active: server, 3d ago)  [review]
  [ ] pm        (last active: laptop, 5d ago)  [planning]
```

Capabilities come from agent_descriptor events in shared/machines/*/agent-state.jsonl.

**Tests required**:
- Test shared/ detection triggers join flow
- Test agent list is populated from descriptors
- Test selected agents are registered locally
- Test database is rebuilt after join

## Acceptance Criteria

- Join flow implemented
- Agent selection UI works
- Selected agents registered locally
- Database rebuilt after join
- Unit tests pass
- go test ./... passes
- Changes committed

