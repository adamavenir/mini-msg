---
id: fray-frz
status: open
deps: []
links: []
created: 2025-12-09T19:04:34.609119-08:00
type: task
priority: 2
---
# Handling long messages in fray chat

**Needs design revisit** given new TUI capabilities.

Original scope:
1. Give more acceptable line length for `fray chat` than for agents. Keep as-is for agents, but set to something huge like 500 lines for users.
2. Give user some way of showing the rest of the message -- either via command, keyboard shortcut, or rewriting full history in view.

New considerations:
- Thread view vs room view behavior
- Interaction with message expansion/collapse
- Mobile-friendly truncation?
- How does this work with structured message display?

First step: Design session to clarify UX.


