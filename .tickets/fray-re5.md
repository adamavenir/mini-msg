---
id: fray-re5
status: closed
deps: []
links: []
created: 2025-12-04T14:03:26.68534-08:00
type: bug
priority: 0
---
# Validate linked project DB exists before opening

## Problem
When opening a linked project database (src/commands/shared.ts:38), there's no validation that the file exists:

```typescript
const linkedDb = new Database(linkedProj.path);
```

`better-sqlite3` silently creates a new empty database if the path doesn't exist. This means:
- If the linked project's DB is moved/deleted, a new empty DB is created
- Silent data divergence - agent thinks they're in the right project
- The stored path is the `.db` file, not project root, making it fragile to renames

## Additional Issues
- `link.ts:34` stores `targetProject.dbPath` (the DB file path)
- `shared.ts:44` sets `project.root` to the DB path, not the project root

## Fix
1. Check `fs.existsSync(linkedProj.path)` before `new Database()`
2. Consider storing project root instead of DB path, and re-discovering
3. Surface clear error: "Linked project 'foo' database not found at /path. Use 'bdm unlink foo' and re-link."

## Files
- src/commands/shared.ts:24-45
- src/commands/link.ts:33-39

## Acceptance Criteria
- Error thrown if linked DB doesn't exist
- Clear error message with remediation steps
- No silent DB creation


