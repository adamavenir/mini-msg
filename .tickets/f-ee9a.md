---
id: f-ee9a
status: closed
deps: [f-79de]
links: []
created: 2026-01-24T05:05:46Z
type: task
priority: 1
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase2]
---
# Phase 2: Machine ID collision detection at init

Implement machine ID collision detection when joining existing multi-machine project.

**Context**: Read docs/MULTI-MACHINE-SYNC-SPEC.md Machine ID Collision section.

**Files to modify**: internal/command/init.go

**Scenario**: User runs 'fray init' in a directory with existing shared/ (synced from another machine).

**Detection**:
1. Read all existing machine directories from shared/machines/
2. Check if proposed machine ID conflicts
3. If conflict: prompt user to choose a different ID

**Example flow**:
```
$ fray init

Found existing fray channel...
  Channel: cool-thing (ch-abc123)
  Machines: laptop, server

Checking machine ID...
  ✗ "laptop" already exists!

Choose a different machine ID:
  Machine ID: laptop2

  ✓ "laptop2" is unique
```

**Tests required**:
- Test collision is detected
- Test user is prompted for new ID
- Test unique ID is accepted
- Test init proceeds after resolution

## Acceptance Criteria

- Collision detection on init implemented
- User prompted for alternate ID
- Init proceeds after resolution
- Unit tests pass
- go test ./... passes
- Changes committed

