---
id: f-d7c3
status: closed
deps: []
links: []
created: 2026-01-25T05:38:34Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-9a7e
---
# Test prep: jsonl_rebuild integration coverage

Add integration tests for RebuildDatabaseFromJSONL prior to refactor.

Plan
- Build a temp .fray dir with representative JSONL for messages/threads/agents/questions/reactions/roles/permissions.
- Run RebuildDatabaseFromJSONL into a temp sqlite DB.
- Assert key invariants: counts, thread membership, pinned/muted states, reaction rows.
- Include a legacy-only fixture and a merged-only fixture.

Keep tests independent of refactor.

## Acceptance Criteria

Integration tests validate rebuild across legacy and merged JSONL.
Tests: go test ./internal/db/...
Commit: test(db): add jsonl_rebuild integration tests


## Notes

**2026-01-25T06:03:43Z**

Added rebuild integration tests in internal/db/jsonl_rebuild_extra_test.go. Tests: go test ./internal/db/... Commit: test(db): add jsonl_rebuild integration tests (197da4b)
