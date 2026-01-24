---
id: fray-nxv
status: closed
deps: []
links: []
created: 2025-12-30T16:46:47.705933-08:00
type: bug
priority: 0
---
# Fix chat sidebar UX + thread search

Current issues:
- Sidebar tries to show channels AND threads simultaneously
- Can't search for threads not in list

Needed behavior:
- Tab cycles: channels → threads → hidden (clean separation)
- Consistent keyboard navigation
- **Thread search**: Type to filter/search threads even if not listed
- Enter to switch to selected item

Reference: internal/chat/model.go


