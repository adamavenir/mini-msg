---
id: fray-kit
status: closed
deps: []
links: []
created: 2025-12-04T09:55:51.813944-08:00
type: epic
priority: 0
---
# Phase 1: Project Setup

Scaffold TypeScript project with build tooling.

## Goal
A working TypeScript project structure with:
- npm package configured for ESM (ECMAScript modules)
- TypeScript compilation via tsup (a fast TS bundler)
- CLI entry point that responds to --version
- Directory structure ready for implementation

## Context
beads-messenger (bdm) is a lightweight agent coordination layer extending beads (the `bd` CLI tool). It provides room-based messaging with @mention routing for AI coding agents working on the same codebase.

Key concepts:
- **Room**: Each beads project has one shared message room
- **Agents**: AI or human participants with hierarchical IDs like alice.1, pm.5.sub.2
- **@mentions**: Route messages to specific agents via prefix matching

See PLAN.md for full design decisions and rationale.

## Exit Criteria
- `npm run build` succeeds
- `bdm --version` works
- Project structure matches spec in PLAN.md


