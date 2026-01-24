---
id: fray-bcx
status: closed
deps: []
links: []
created: 2025-12-31T08:01:09.723069-08:00
type: feature
priority: 2
---
# Enhance UserPromptSubmit hook with status line

Add compact status line to UserPromptSubmit hook:
- Asked questions count
- Wondered questions count  
- Unread count (once summary command exists)
- Claims status (all agents, for collision awareness)

Format: `[fray opus] asked:2 wondered:1 | claims: @alice (auth.go)`

Files: internal/command/hook_prompt.go


