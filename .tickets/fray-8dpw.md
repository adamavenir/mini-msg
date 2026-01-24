---
id: fray-8dpw
status: open
deps: []
links: []
created: 2025-12-31T23:53:31.201175-08:00
type: task
priority: 3
---
# Extract hook commands to separate package

Move 5 hook-related commands (hook_*.go, 830+ lines total) from internal/command/ to internal/command/hooks/ subdirectory. These are infrastructure for Claude Code integration, not user-facing commands.


