---
id: fray-eya
status: closed
deps: []
links: []
created: 2025-12-21T10:56:33.792706-08:00
type: bug
priority: 1
---
# Bug: prune --all doesn't prune all, leaves 20

Current behavior:
- --all leaves last 20 messages (should prune ALL)
- Default keeps 100 messages

Expected behavior:
- --all should prune ALL messages (0 remaining)
- Default should keep last 20 (not 100)


