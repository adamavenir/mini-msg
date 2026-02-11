---
id: f-4d0d
status: open
deps: [f-2caa]
links: []
created: 2026-01-25T11:09:28Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-d1ae
---
# Phase 6: Split daemon spawn module

Split internal/daemon/daemon_spawn.go into focused modules.

Suggested file layout:
- internal/daemon/daemon_spawn_cmd.go: spawnAgent/spawnBRBAgent top-level orchestration.
- internal/daemon/daemon_spawn_prompts.go: buildContinuationPrompt/buildWakePrompt/buildInlinePrompt + build*PromptInline helpers.
- internal/daemon/daemon_spawn_templates.go: executePromptTemplate + template helpers.
- internal/daemon/daemon_spawn_monitor.go: monitorProcess/handleProcessExit/killProcess.
- internal/daemon/daemon_spawn_messages.go: getMessagesAfter/getSessionMessages/runStdoutRepair.
- internal/daemon/daemon_spawn_interrupt.go: getInterruptInfo/handleInterrupt/clear cooldown helpers.
- internal/daemon/daemon_spawn_helpers.go: detectSpawnMode/getDriver/getAgentResolution + misc helpers.

Preserve daemon behavior and concurrency semantics.

## Acceptance Criteria

daemon_spawn.go split into focused files; behavior unchanged. go test ./internal/daemon passes. Commit with message like 'refactor(daemon): split spawn module'.

