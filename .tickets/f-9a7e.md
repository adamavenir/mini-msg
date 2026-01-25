---
id: f-9a7e
status: open
deps: []
links: []
created: 2026-01-25T05:32:58Z
type: epic
priority: 2
assignee: Adam Avenir
---
# Refactor: split large files across db/daemon/commands/cli

Context
- Reduce file sizes by splitting large Go files without behavior changes.
- Target files: internal/db/jsonl_read.go, internal/db/jsonl_append.go, internal/db/jsonl_rebuild.go, internal/db/jsonl.go, internal/daemon/daemon.go, internal/chat/commands.go, internal/command/agent.go, internal/command/thread_container.go, internal/command/get.go, internal/db/schema.go, cmd/libfray/main.go.

Guidelines
- Preserve public APIs and package boundaries; move code only.
- Add minimal shared helpers (e.g., jsonl_*_common.go) rather than duplicating.
- Keep file names explicit to the domain (messages/threads/agents/etc).
- Keep gofmt clean; no functional changes.

Testing/Commits
- Each phase runs tests and commits before the next phase (see child tickets).
- Prefer targeted tests (go test ./internal/db/..., ./internal/chat/..., etc) and a final go test ./... if reasonable.

## Acceptance Criteria

All child tickets closed.
Each phase includes a test run + commit noted in its ticket.
No behavioral changes; build/tests pass.

