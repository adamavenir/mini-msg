---
id: fray-0wr
status: closed
deps: []
links: []
created: 2025-12-22T12:09:15.846479-08:00
type: bug
priority: 1
---
# Bug: /prune --all does not prune all messages

The --all flag should prune ALL messages (0 remaining), but currently leaves some behind. Fix the prune logic so --all truly removes everything.


