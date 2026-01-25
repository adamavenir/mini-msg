---
id: f-08db
status: open
deps: [f-a53c]
links: []
created: 2026-01-25T05:33:39Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-9a7e
---
# Phase 5: Split internal/chat/commands.go

Split slash-command handling into families and a small dispatcher.

Suggested files
- internal/chat/commands_dispatch.go: entrypoints + command lookup.
- internal/chat/commands_threads.go: /thread, /subthread, /rename, /mv thread, navigation.
- internal/chat/commands_messages.go: /edit, /delete, /pin, /unpin, /close, /prune, reply helpers.
- internal/chat/commands_agents.go: /bye, /fly, /hop, /land.
- internal/chat/commands_mlld.go: /run and script listing/output parsing.
- internal/chat/commands_helpers.go: parsing utilities, shared helpers.

Keep behavior and tests unchanged.

## Acceptance Criteria

commands.go split into focused files.
Tests: go test ./internal/chat/...
Commit: refactor(chat): split commands by family

