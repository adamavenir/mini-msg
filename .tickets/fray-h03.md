---
id: fray-h03
status: closed
deps: []
links: []
created: 2025-12-21T10:51:34.105328-08:00
type: feature
priority: 2
---
# Add mm destroy <channel> to delete channel entirely

mm destroy <channel> should:
- Remove the entry from global config (~/.config/mm/mm-config.json)
- Delete the .mm/ directory for that channel

Requires N/y confirmation (default No) with clear warning that this is irreversible and destroys all messages/history. Use --force to skip confirmation.


