---
id: f-d1ae
status: open
deps: []
links: []
created: 2026-01-25T11:07:47Z
type: epic
priority: 2
assignee: Adam Avenir
---
# Refactor: split remaining large files (commands/chat/daemon/hooks)

Scope:
- internal/command/prune.go
- internal/command/thread_curation.go
- internal/command/migrate.go
- internal/command/init.go
- internal/chat/commands_thread.go
- internal/daemon/daemon_spawn.go
- internal/command/hooks/hook_safety.go

Goals:
- Split by responsibility (cmd wiring vs domain helpers vs IO) with minimal behavior changes.
- Add stability tests before each refactor.
- Each phase: add tests -> refactor -> run targeted go test -> commit before proceeding.

Conventions:
- Keep public APIs and CLI behavior stable.
- Prefer small, focused files named by responsibility (e.g., *_cmd.go, *_helpers.go, *_io.go).
- Update imports and keep Go fmt clean.

Phase map (each refactor depends on its test ticket; phases are sequenced via deps):
- Phase 1 (prune): tests f-82d6 -> refactor f-818d
- Phase 2 (thread curation): tests f-0363 -> refactor f-6e39
- Phase 3 (migrate): tests f-7324 -> refactor f-3c7a
- Phase 4 (init): tests f-7056 -> refactor f-7648
- Phase 5 (chat thread commands): tests f-966c -> refactor f-9d93
- Phase 6 (daemon spawn): tests f-2caa -> refactor f-4d0d
- Phase 7 (hook safety): tests f-65da -> refactor f-069a

## Acceptance Criteria

All phase tickets complete; package tests pass per phase; final go test ./... passes.
