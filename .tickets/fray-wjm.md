---
id: fray-wjm
status: closed
deps: [fray-kit, fray-bml]
links: []
created: 2025-12-04T09:58:24.845618-08:00
type: task
priority: 1
parent: fray-08v
---
# Implement beads database discovery

Create src/core/project.ts to discover and connect to the beads database.

## Behavior
1. Start from current working directory
2. Walk up directory tree looking for `.beads/` directory
3. Find `*.db` file within `.beads/`
4. Return absolute path to the database file

## Error Cases
- No `.beads/` directory found: Exit with error "Not in a beads project. Run 'bd init' first."
- Multiple `*.db` files: Use first one (alphabetically) or error if ambiguous
- `.beads/` exists but no `*.db`: Exit with error "Beads project not initialized. Run 'bd init' first."

## API
```typescript
// src/core/project.ts

export interface BeadsProject {
  root: string;      // Absolute path to project root
  dbPath: string;    // Absolute path to .beads/*.db
}

/**
 * Discover beads project from current directory.
 * Walks up the directory tree looking for .beads/
 * @throws Error if not in a beads project
 */
export function discoverProject(startDir?: string): BeadsProject;

/**
 * Open database connection to beads project.
 * Initializes bdm schema if not present.
 */
export function openDatabase(project: BeadsProject): Database;
```

## Files
- src/core/project.ts

## Acceptance Criteria
- Discovers .beads/ from any nested subdirectory
- Returns correct absolute paths
- Clear error messages when not in beads project
- Unit tests for discovery logic


