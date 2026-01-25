---
id: f-24fd
status: open
deps: [f-7233]
links: []
created: 2026-01-25T05:33:50Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-9a7e
---
# Phase 7: Split internal/db/schema.go

Split schema definitions and migrations into smaller files.

Suggested files
- internal/db/schema_core.go: open/init helpers, shared constants.
- internal/db/schema_migrations.go: migration list + runner.
- internal/db/schema_tables_*.go: per-domain DDL (messages/threads/agents/roles/reactions/permissions/jobs/etc).

Keep migration ordering identical.

## Acceptance Criteria

schema.go split into core/migrations/domain tables.
Tests: go test ./internal/db/...
Commit: refactor(db): split schema definitions

