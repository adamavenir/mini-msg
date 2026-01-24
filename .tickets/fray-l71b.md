---
id: fray-l71b
status: open
deps: []
links: []
created: 2025-12-31T23:03:07.602932-08:00
type: feature
priority: 2
---
# Update fly/land protocols: workstream threads

fly: Create thread for agent's workstream on session start (when appropriate)
land: Collapse and close workstream thread so it can be referenced

Design consideration: Not all sessions need threads. Simple 'complete task a, b, c' sessions where the plan is clear might just update beads. Need heuristics for when to create workstream thread vs. not.

Potential triggers for workstream thread:
- Exploratory/design work
- Multi-step implementation with decisions
- Work that might inform future sessions

Skip thread for:
- Clear task execution with known approach
- Bug fixes with obvious solution
- Chore/cleanup work


