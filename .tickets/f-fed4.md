---
id: f-fed4
status: closed
deps: []
links: []
created: 2026-01-25T05:38:22Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-9a7e
---
# Test prep: jsonl_read regression coverage

Add focused tests for JSONL read/merge behavior before refactor.

Plan
- Add table-driven tests for ReadMessages/ReadThreads/ReadAgents/ReadQuestions/ReadReactions/ReadRoles/ReadPermissions.
- Use temp .fray dirs with mixed legacy + merged JSONL lines.
- Cover update/delete semantics, timestamp ordering, and dedupe.
- Add golden fixtures for message updates/reactions, thread updates/deletes, and permission status transitions.

Keep tests independent of refactor; no behavior changes.

## Acceptance Criteria

New tests cover each Read* path and merge/legacy cases.
Tests: go test ./internal/db/...
Commit: test(db): add jsonl_read regression coverage


## Notes

**2026-01-25T06:03:32Z**

Added jsonl_read regression tests in internal/db/jsonl_read_additional_test.go. Tests: go test ./internal/db/... Commit: test(db): add jsonl_read regression coverage (c245a19)
