---
id: fray-3iq
status: closed
deps: [fray-3vp]
links: []
created: 2025-12-19T13:13:41.599699-08:00
type: task
priority: 1
---
# P1.7: Add time-based query flags

Add time-based query flags for context-aware message retrieval.

New flags for mm get, mm history, mm between:
- --since <time|guid>: Messages after this point
- --before <time|guid>: Messages before this point
- --from <time|guid> --to <time|guid>: Range query

Time formats:
- Relative: 1h, 2d, 30m, 1w
- Absolute: yesterday, today
- GUID: @#a1b2 or @#msg-a1b2c3d4

Examples:
mm get --since 1h
mm history alice --since yesterday
mm history alice --since @#a1b2
mm get --from @#a1b2 --to @#x9y8

Implementation:
- src/core/time-query.ts (NEW): Parse time expressions
  - parseTimeExpression("1h") → Date
  - parseTimeExpression("@#a1b2") → GUID → created_at
- Update query builders in src/db/queries.ts
- Add WHERE created_at >= ? or created_at <= ?
- Add WHERE created_at BETWEEN ? AND ?

Time parsing:
- 1h → Date(now - 1 hour)
- 2d → Date(now - 2 days)
- yesterday → Date(start of yesterday)
- @#a1b2 → resolve GUID → lookup created_at

References: PLAN.md section 9
Critical files: src/core/time-query.ts (NEW), src/db/queries.ts, src/commands/get.ts, src/commands/history.ts


