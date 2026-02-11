---
id: f-05ba
status: closed
deps: []
links: []
created: 2026-01-25T05:38:40Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-9a7e
---
# Test prep: daemon split safety net

Add tests around daemon subsystems before splitting daemon.go.

Plan
- Add unit tests for lock acquisition/release and stale lock handling.
- Add tests for isActiveByTokens + spawn/queue decision helpers.
- Add tests for cleanupStalePresence and presence debounce logic.
- Add tests for usage snapshot persistence and watcher state transitions (using temp dirs).

Keep tests independent of refactor.

## Acceptance Criteria

New daemon tests cover lock, spawn decision, presence cleanup, usage snapshot paths.
Tests: go test ./internal/daemon/...
Commit: test(daemon): add split safety net


## Notes

**2026-01-25T06:03:47Z**

Added daemon prereq tests in internal/daemon/daemon_prereq_test.go. Tests: go test ./internal/daemon/... Commit: test(daemon): add split safety net (fa29c85)
