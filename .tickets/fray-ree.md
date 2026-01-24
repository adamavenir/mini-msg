---
id: fray-ree
status: closed
deps: [fray-3vp, fray-qqv]
links: []
created: 2025-12-19T13:31:02.571331-08:00
type: task
priority: 0
---
# P0.8: Tests for GUID system

Add comprehensive tests for GUID-based architecture.

Test files to create/update:

1. tests/guid.test.ts (NEW)
   - GUID generation (uniqueness, format, length)
   - Base58 encoding validation
   - Type prefixes (msg-, usr-, ch-)
   - Collision handling

2. tests/jsonl.test.ts (NEW)
   - appendMessage() writes valid JSONL
   - readMessages() parses correctly
   - Handle malformed JSONL (skip bad lines)
   - Append-only guarantees
   - File locking behavior (optional)

3. tests/schema.test.ts (UPDATE)
   - GUID columns exist and are primary keys
   - No short_id or display_id columns
   - Channel_id foreign key relationships

4. tests/queries.test.ts (NEW)
   - Prefix matching resolution
   - Ambiguous GUID handling
   - Channel-scoped queries
   - GUID → message lookup

5. tests/rebuild.test.ts (NEW)
   - SQLite rebuilds from JSONL
   - Staleness detection works
   - GUIDs preserved across rebuilds
   - Handles missing JSONL files

Coverage goals:
- Core GUID functions: 100%
- JSONL writers/readers: 95%
- Schema migrations: 90%
- Query resolvers: 95%

Run with: npm test

References: Existing tests in tests/
Critical files: tests/guid.test.ts (NEW), tests/jsonl.test.ts (NEW), tests/rebuild.test.ts (NEW)


