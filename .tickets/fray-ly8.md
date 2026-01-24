---
id: fray-ly8
status: closed
deps: []
links: []
created: 2025-12-04T14:03:48.663946-08:00
type: task
priority: 1
---
# Add SQLite WAL mode and busy_timeout for multi-agent resilience

## Problem
No WAL mode or busy_timeout configured when opening databases (src/core/project.ts:55-60, src/commands/shared.ts:38-40).

```typescript
export function openDatabase(project: BeadsProject): Database.Database {
  const db = new Database(project.dbPath);
  db.pragma('foreign_keys = ON');
  return db;  // No WAL, no busy_timeout
}
```

Multiple agents posting/watching simultaneously can hit SQLITE_BUSY errors. This is the primary use case for bdm!

## Fix
Add pragmas after opening:
```typescript
db.pragma('journal_mode = WAL');
db.pragma('busy_timeout = 5000');  // 5 seconds
```

## Considerations
- WAL mode creates additional files (*.db-wal, *.db-shm)
- beads itself may have opinions on journal mode - check compatibility
- busy_timeout of 5000ms is reasonable; can make configurable later

## Files
- src/core/project.ts:55-60
- src/commands/shared.ts:38-40 (linked project opening)

## Acceptance Criteria
- Database opened with WAL mode enabled
- busy_timeout set to reasonable value (5000ms)
- Multiple concurrent bdm processes don't get SQLITE_BUSY


