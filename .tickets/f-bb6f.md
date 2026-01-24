---
id: f-bb6f
status: closed
deps: [f-79de]
links: []
created: 2026-01-24T05:01:56Z
type: task
priority: 1
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase1]
---
# Phase 1: Mention encoding with machine scope and alias resolution

Implement mention encoding that adds machine scope at write time, with alias resolution for renamed machines.

**Context**: Read docs/MULTI-MACHINE-SYNC-PLAN.md section 1.4 and SPEC section on Mention Encoding.

**Files to modify**: internal/core/mentions.go (or similar)

**Mention encoding**:
- @opus → @opus@{local-machine} at write time
- @opus@server → @opus@server (keep explicit machine)
- @opus@all → @opus@all (broadcast)

**Alias resolution** (for future machine rename):
```go
func encodeMention(config Config, mention string, localMachine string) string {
    if !hasMachineQualifier(mention) {
        return mention + "@" + localMachine
    }
    // Resolve aliases for renamed machines only (not retired)
    parts := parseMention(mention)
    if alias, ok := config.MachineAliases[parts.Machine]; ok && alias != "" {
        parts.Machine = alias
    }
    return parts.String()
}
```

**Retired machine handling**:
- Keep original ID in storage (don't rewrite to empty)
- isRetiredMachine() helper for daemon to skip processing

**Tests required**:
- Test bare @opus becomes @opus@laptop
- Test explicit @opus@server stays unchanged
- Test alias resolution rewrites old-laptop to new-laptop
- Test retired machines keep original ID

## Acceptance Criteria

- Mention encoding adds machine scope
- Alias resolution works for renamed machines
- Retired machines keep original ID
- Unit tests pass
- go test ./... passes
- Changes committed

