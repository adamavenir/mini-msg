---
id: fray-dro
status: closed
deps: []
links: []
created: 2025-12-22T12:34:24.19896-08:00
type: feature
priority: 2
---
# mm @agent: show activity summary and claims hint

When running mm @agent, after showing mentions, display a summary to help agents catch up:

---
X messages since you last posted (from @bob, @charlie)
Active claims: @bob (src/auth.ts), @pm (docs/)
Run 'mm get <agent>' to catch up

This nudges agents toward the right command and shows claim awareness without changing core behavior.


