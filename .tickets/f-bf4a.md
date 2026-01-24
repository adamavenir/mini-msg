---
id: f-bf4a
status: closed
deps: [f-79de]
links: []
created: 2026-01-24T05:08:21Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase5]
---
# Phase 5: fray machines command

Implement fray machines command to list all machines in the channel.

**Context**: Read docs/MULTI-MACHINE-SYNC-SPEC.md CLI Commands section.

**Files to create**: internal/command/machines.go

**Commands**:
```bash
fray machines                    # List all machines
fray machines --verbose          # Show file counts, last activity
```

**Output format**:
```
MACHINE     STATUS    LAST WRITE    AGENTS
laptop      local     2 min ago     opus, designer
server      synced    15 min ago    opus, reviewer
desktop     synced    3 hours ago   designer
```

**Status values**:
- local: this machine
- synced: remote machine with recent activity
- stale: remote machine with no recent activity (>7 days)

**Verbose mode**: Show message/thread counts per machine.

**Tests required**:
- Test machine list is populated
- Test status is correct for local vs remote
- Test verbose mode shows counts

## Acceptance Criteria

- fray machines command implemented
- Status shown for each machine
- Verbose mode works
- Unit tests pass
- go test ./... passes
- Changes committed

