---
id: fray-yh4
status: closed
deps: [fray-dk4]
links: []
created: 2025-12-04T10:07:42.21857-08:00
type: epic
priority: 0
---
# Phase 4: CLI Commands

Implement all CLI commands for bdm.

## Goal
Complete CLI implementation with all commands from the spec:
- Lifecycle: new, hi, bye
- Presence: here, who
- Messages: (default), @mentions, post, --watch
- Cross-project: link, unlink, projects
- Config: config

## Context
CLI uses Commander.js for parsing. Each command is in a separate file under src/commands/.
Global --project flag operates in linked project context.

See PLAN.md "CLI Specification" section for full command details.

## Exit Criteria
- All commands implemented per spec
- `--help` works for all commands
- Error messages are clear
- Integration tests for key flows


