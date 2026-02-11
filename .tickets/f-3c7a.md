---
id: f-3c7a
status: open
deps: [f-7324]
links: []
created: 2026-01-25T11:08:44Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-d1ae
---
# Phase 3: Split migrate command

Split internal/command/migrate.go into focused modules.

Suggested file layout:
- internal/command/migrate_cmd.go: NewMigrateCmd + flag parsing + top-level flow.
- internal/command/migrate_legacy_tables.go: tableExists/getColumns/columnsInclude.
- internal/command/migrate_agents.go: agentRow types, loadAgents/scanAgents, agent conversion.
- internal/command/migrate_messages.go: messageRow types, loadMessages/scanMessages, mention parsing.
- internal/command/migrate_receipts.go: readReceiptRow + load/restoreReadReceipts.
- internal/command/migrate_helpers.go: generateUniqueGUID/containsGUID/resolveReply*/null* helpers.
- internal/command/migrate_fs.go: writeJSONLFile/copyDir/copyFile.

Keep public behavior identical; no format changes to JSONL outputs.

## Acceptance Criteria

migrate.go split into focused files; behavior unchanged. go test ./internal/command passes. Commit with message like 'refactor(command): split migrate command'.

