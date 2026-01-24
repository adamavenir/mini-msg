---
id: fray-w7oo
status: open
deps: []
links: []
created: 2025-12-31T23:53:31.057154-08:00
type: epic
priority: 2
---
# Split db/jsonl.go by operation type

db/jsonl.go is 1376 lines. Split into: jsonl_append.go (11 append functions), jsonl_read.go (4 read functions + version history), jsonl_rebuild.go (RebuildDatabaseFromJSONL). Cleaner separation of write-only, read-only, and rebuild operations.


