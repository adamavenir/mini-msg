---
id: f-d0cf
status: closed
deps: [f-bb6f, f-bf4a]
links: []
created: 2026-01-24T05:08:43Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase5]
---
# Phase 5: fray machine rename command

Implement fray machine rename command with alias preservation.

**Context**: Read docs/MULTI-MACHINE-SYNC-PLAN.md section 5.3 and SPEC Machine Rename/Retire section.

**Files to modify**: internal/command/machines.go

**Command**:
```bash
fray machine rename <old> <new>
```

**Steps**:
1. Add entry to machine_aliases in fray-config.json
2. Rename shared/machines/<old>/ to shared/machines/<new>/
3. Update local/machine-id if renaming local machine
4. Rebuild database

**Alias map in fray-config.json**:
```json
{
  "machine_aliases": {
    "old-laptop": "new-laptop"
  }
}
```

**Prerequisite**: Alias resolution already wired in Phase 1 (mention encoding).

All existing mentions using old ID continue to work via alias resolution.

**Tests required**:
- Test rename updates alias map
- Test directory is renamed
- Test old mentions still resolve
- Test local machine rename updates machine-id

## Acceptance Criteria

- Machine rename command implemented
- Alias map updated
- Directory renamed
- Old mentions work via alias
- Unit tests pass
- go test ./... passes
- Changes committed

