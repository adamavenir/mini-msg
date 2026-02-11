---
id: f-2caa
status: open
deps: [f-9d93]
links: []
created: 2026-01-25T11:09:19Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-d1ae
---
# Phase 6: Add tests for daemon spawn logic

Add coverage for spawn logic in internal/daemon/daemon_spawn.go before refactor.

Test targets (suggested):
- detectSpawnMode: routing for inline/wake/fly/hand/hop/land/resume markers.
- build*PromptInline helpers: ensure template snippets include trigger info/user message.
- buildContinuationPrompt: includes recent context + system hints.
- runStdoutRepair: returns stable output for known inputs.

Favor pure/helper tests that avoid running external processes; use existing daemon test harness where needed.

## Acceptance Criteria

New tests in internal/daemon (e.g., daemon_spawn_test.go) cover spawn mode detection + prompt builders. go test ./internal/daemon -run Spawn passes. Commit with message like 'test(daemon): add spawn coverage'.

