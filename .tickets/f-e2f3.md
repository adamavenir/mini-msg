---
id: f-e2f3
status: open
deps: [f-071a, f-d7c3]
links: []
created: 2026-01-25T05:33:25Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-9a7e
---
# Phase 3: Split internal/db/jsonl_rebuild.go and jsonl.go records

Split rebuild pipeline and JSONL record structs into per-entity files.

Suggested files
- internal/db/jsonl_records_common.go: file constants and shared record types.
- internal/db/jsonl_records_messages.go, _threads.go, _agents.go, _questions.go, _permissions.go, _reactions.go, _roles.go, _wake.go, _jobs.go.
- internal/db/jsonl_rebuild_core.go: RebuildDatabaseFromJSONL orchestrator + shared helpers.
- internal/db/jsonl_rebuild_messages.go, _threads.go, _agents.go, _questions.go, _reactions.go, _roles.go, _permissions.go, _wake.go, _jobs.go.

Keep behavior identical.

## Acceptance Criteria

jsonl.go record structs moved into entity files.
Rebuild logic split by entity; core orchestrator remains.
Tests: go test ./internal/db/...
Commit: refactor(db): split jsonl_rebuild + records

