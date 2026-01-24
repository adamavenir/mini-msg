---
id: fray-qhy
status: open
deps: []
links: []
created: 2025-12-30T16:14:41.783494-08:00
type: feature
priority: 3
---
# Implement Tasks primitive

Agent work queue with blocking/unblocking.

**Spec**: threads-03-aspirational.md (lines 128-148)

Key features:
- task-XXXXXXXX GUIDs
- Status: pending, active, done, blocked
- External ID linking: bead:xyz-123, gh:owner/repo/123, linear:PROJ-123
- blocked_by: array of question/task GUIDs
- Auto-unblock when blocking question resolves (→ pending, not active)

CLI:
- fray task add "..." --as bob [--external bead:xyz-123]
- fray tasks [@agent]
- fray task active/done/block task-abc

Storage: tasks.jsonl or graph.jsonl


