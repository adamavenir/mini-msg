---
id: fray-sw9d
status: closed
deps: [fray-eg25]
links: []
created: 2025-12-31T23:53:43.098075-08:00
type: task
priority: 2
---
# Extract chat/panels.go from model.go

Extract sidebar + thread panel: renderThreadPanel(), renderSidebar(), handleThreadPanelKeys(), handleSidebarKeys(), threadEntries(), filtering logic, navigation helpers. ~800 lines - biggest extraction.


