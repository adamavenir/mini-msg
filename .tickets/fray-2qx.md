---
id: fray-2qx
status: closed
deps: []
links: []
created: 2025-12-30T23:17:36.807234-08:00
type: task
priority: 2
---
# Filter deleted messages from display

Deleted messages show as '[deleted]' in `fray notes` and message history. Should filter them out for cleaner UX.

Affected commands:
- `fray notes --as <agent>`
- `fray thread <ref>`
- `fray history <agent>`
- Possibly `fray get` and chat view

Implementation: Filter out messages where body == '[deleted]' or add a deleted flag check.


