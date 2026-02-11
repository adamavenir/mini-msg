---
id: f-65da
status: open
deps: [f-4d0d]
links: []
created: 2026-01-25T11:09:36Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-d1ae
---
# Phase 7: Add tests for hook safety install/uninstall

Add coverage for safety hook installation/uninstallation before refactor.

Test targets (suggested):
- installSafetyHooks (project scope): writes guard script + settings.local.json with PreToolUse hook.
- installSafetyHooks (global scope): writes to ~/.claude and uses absolute path.
- uninstallSafetyHooks: removes guard + prunes PreToolUse entries; idempotent when missing.
- uninstallIntegrationHooks: removes integration hooks without touching other settings.
- dry-run output does not mutate filesystem.

Use temp dirs and explicit HOME override for global scope.

## Acceptance Criteria

New tests in internal/command/hooks (e.g., hook_safety_test.go) validate install/uninstall behavior. go test ./internal/command -run HookSafety passes. Commit with message like 'test(command): add hook safety coverage'.

