---
id: f-a53c
status: open
deps: [f-e2f3, f-05ba]
links: []
created: 2026-01-25T05:33:31Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-9a7e
---
# Phase 4: Split internal/daemon/daemon.go

Break daemon into focused files.

Suggested files
- internal/daemon/daemon_core.go: public API, Start/Stop, main loop wiring.
- internal/daemon/daemon_config.go: Config/defaults/options.
- internal/daemon/daemon_lock.go: lock file handling.
- internal/daemon/daemon_poll.go: polling for mentions/reactions/queues.
- internal/daemon/daemon_spawn.go: spawn/queue decisions, process mgmt.
- internal/daemon/daemon_usage.go: usage watcher + snapshot persistence.
- internal/daemon/daemon_presence.go: presence debouncing + cleanup.

Keep exported API stable.

## Acceptance Criteria

daemon.go split into logical files.
Tests: go test ./internal/daemon/...
Commit: refactor(daemon): split daemon into modules

