---
id: fray-c0sc
status: closed
deps: []
links: []
created: 2025-12-31T23:38:06.530795-08:00
type: bug
priority: 2
---
# Chat: filter deleted messages from display

The filterDeletedMessages helper was added to CLI commands (notes, thread, history, etc.) but the fray chat TUI still shows '[deleted]' placeholders for deleted messages.

Need to add filtering in the chat view rendering, similar to internal/chat/commands.go:155 check.


