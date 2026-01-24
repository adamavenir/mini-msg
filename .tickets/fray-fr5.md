---
id: fray-fr5
status: closed
deps: [fray-3po]
links: []
created: 2025-12-04T09:56:29.176972-08:00
type: task
priority: 1
parent: fray-kit
---
# Set up project dependencies

Install and configure all required dependencies for bdm.

## Runtime Dependencies
- better-sqlite3: Synchronous SQLite3 bindings for Node.js
- commander: CLI framework for argument parsing

## Dev Dependencies
- typescript: TypeScript compiler
- tsup: Fast TypeScript bundler
- vitest: Test framework
- @types/better-sqlite3: Type definitions
- @types/node: Node.js type definitions

## Optional (defer if not needed)
- chalk: Terminal colors (respect NO_COLOR env var)

## Tasks
1. Run npm install for each dependency
2. Verify better-sqlite3 compiles native bindings successfully
3. Verify TypeScript recognizes all type definitions

## Notes
- better-sqlite3 requires native compilation - ensure build tools available
- Use exact versions in package.json for reproducibility

## Acceptance Criteria
- All dependencies install without errors
- `import Database from 'better-sqlite3'` compiles
- `import { Command } from 'commander'` compiles


