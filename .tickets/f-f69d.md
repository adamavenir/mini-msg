---
id: f-f69d
status: closed
deps: [f-79de]
links: []
created: 2026-01-24T05:00:24Z
type: task
priority: 1
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase1]
---
# Phase 1: Sequence counter with atomic increment

Implement per-machine sequence counter for deterministic merge ordering.

**Context**: Read docs/MULTI-MACHINE-SYNC-PLAN.md section 2.2 and SPEC section on Machine ID Stability.

**Files to modify**: internal/db/jsonl.go

**Function to add**:
GetNextSequence(projectPath) (int64, error)

**Implementation**:
```go
func GetNextSequence(projectPath string) (int64, error) {
    path := filepath.Join(projectPath, ".fray", "local", "machine-id")
    // flock → read JSON → increment seq → write → fsync → unlock
    // Crash recovery: if corrupt, scan local JSONL for max seq + 1
}
```

**Atomicity**: Use syscall.Flock for exclusive lock during increment.

**Crash recovery**: If machine-id file is corrupt or seq is missing, scan all local JSONL files (shared/machines/{local}/*.jsonl) for maximum seq value and use max+1.

**Tests required**:
- Test sequence increments correctly
- Test concurrent calls are serialized (flock)
- Test crash recovery: corrupt file → scan → correct seq
- Test crash recovery: missing seq field → scan → correct seq

## Acceptance Criteria

- GetNextSequence implemented with flock
- Crash recovery logic implemented
- Unit tests for atomicity and recovery
- go test ./... passes
- Changes committed

