---
id: f-bb43
status: closed
deps: [f-2312]
links: []
created: 2026-01-24T05:07:34Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase4]
---
# Phase 4: fsnotify watcher for auto-rebuild

Implement file system watching for automatic database rebuild on sync.

**Context**: Read docs/MULTI-MACHINE-SYNC-PLAN.md Phase 4.

**Files to modify**: internal/command/daemon.go (or similar)

**Implementation**:
- Use fsnotify to watch shared/machines/*/*.jsonl
- On file change, trigger database rebuild
- Debounce rapid changes (wait 500ms for batch)

**Daemon integration**:
```bash
fray daemon --watch    # Start daemon with file watching
```

**Watch paths**:
- shared/machines/**/messages.jsonl
- shared/machines/**/threads.jsonl
- shared/machines/**/questions.jsonl
- shared/machines/**/agent-state.jsonl

**Tests required**:
- Test file change triggers rebuild
- Test debouncing works
- Test new machine directory is detected

## Acceptance Criteria

- fsnotify watching implemented
- Auto-rebuild on file changes
- Debouncing for rapid changes
- Unit tests pass
- go test ./... passes
- Changes committed

