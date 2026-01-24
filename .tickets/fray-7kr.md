---
id: fray-7kr
status: closed
deps: []
links: []
created: 2025-12-19T11:57:08.832023-08:00
type: task
priority: 0
---
# P0.1: Add GUID columns to schema

Add guid TEXT PRIMARY KEY to mm_messages and mm_agents tables. Generate short GUIDs using 8-char base58 encoding. Remove any existing short_id or display_id columns (GUID-only architecture).

Implementation:
- Update src/db/schema.ts to add guid column
- Add GUID generation function (base58, 8 chars)
- Create migration to populate GUIDs for existing rows
- Drop short_id/display_id columns if they exist

References: PLAN.md sections 1, 2
Critical file: src/db/schema.ts


