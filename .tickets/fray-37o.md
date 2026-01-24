---
id: fray-37o
status: closed
deps: []
links: []
created: 2025-12-30T16:47:02.578266-08:00
type: task
priority: 2
assignee: designer
---
# Design: pin vs star semantics

Two distinct concepts conflated in 'pin':

**Pin** = Signal importance to everyone
- 'This is important for the channel/thread'
- Shows in pinned section at top
- Visibility: all agents see it

**Star** = Personal subscription/bookmark  
- 'I want to track this'
- Shows in my threads list
- Example: starring opus-notes to have it listed

Questions:
- Are these separate commands? `fray pin` vs `fray star`?
- Can you pin someone else's message? Star only your own subscriptions?
- How does this interact with thread membership?
- UI: pinned section vs starred sidebar

Needs design before implementing either.


