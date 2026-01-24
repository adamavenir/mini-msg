---
id: f-b3bd
status: closed
deps: [f-a390, f-366d]
links: []
created: 2026-01-24T05:04:30Z
type: task
priority: 1
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase2]
---
# Phase 2: Tombstone events for all types

Implement tombstone events for all deletable entity types.

**Context**: Read docs/MULTI-MACHINE-SYNC-SPEC.md JSONL Format sections on Messages and Threads.

**Files to modify**: 
- internal/db/jsonl_append.go (new event types)
- internal/db/jsonl_read.go (tombstone tracking in merge)
- internal/db/jsonl_rebuild.go (apply tombstones)

**Tombstone event types**:
- message_delete - sticky message deletion
- thread_delete - sticky thread deletion  
- fave_remove - unfave thread/message
- role_release - release held role
- cursor_clear - clear ghost cursor

**Example events**:
```jsonl
{"type":"message_delete","id":"msg-abc","deleted_by":"opus","seq":4,"ts":...}
{"type":"thread_delete","thread_id":"thrd-abc","seq":5,"ts":...}
{"type":"fave_remove","agent_id":"opus","item_guid":"thrd-abc","seq":6,"ts":...}
{"type":"role_release","agent_id":"opus","role_name":"architect","seq":7,"ts":...}
{"type":"cursor_clear","agent_id":"opus","home":"room","seq":8,"ts":...}
```

**Rebuild logic**:
- Build deletion set from tombstones first
- Skip deleted items when building tables
- Tombstones are sticky: once deleted anywhere, deleted everywhere

**Tests required**:
- Test each tombstone type is recorded
- Test rebuild skips tombstoned items
- Test tombstones from any machine are respected

## Acceptance Criteria

- All tombstone event types implemented
- Append functions for each tombstone type
- Rebuild respects tombstones
- Unit tests pass
- go test ./... passes
- Changes committed

