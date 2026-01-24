---
id: fray-3rq
status: closed
deps: []
links: []
created: 2025-12-21T20:33:37.763771-08:00
type: feature
priority: 2
---
# Chat: enable text selection and copy support

Currently cannot select text in chat TUI for copy/paste. Option+drag doesn't work.

Options to investigate:
1. Check if mouse capture is interfering with native terminal selection
2. Implement custom text selection within the TUI
3. Add a 'copy mode' (like tmux) that disables mouse capture temporarily
4. Support modifier key bypass for native selection

Goal: users should be able to copy message text easily.


