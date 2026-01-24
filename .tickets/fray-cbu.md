---
id: fray-cbu
status: closed
deps: []
links: []
created: 2025-12-21T01:28:36.28564-08:00
type: bug
priority: 0
---
# Go: Fix JSON output to use snake_case keys

Go uses PascalCase (ID, TS, ChannelID) but TS uses snake_case (id, ts, channel_id). This breaks API compatibility.


