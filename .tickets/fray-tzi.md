---
id: fray-tzi
status: closed
deps: []
links: []
created: 2025-12-21T12:13:23.975689-08:00
type: feature
priority: 2
---
# Show status/purpose consistently across roster, info, who

Display status and purpose fields consistently in roster, info, and who commands.

When not set:
- JSON output: show as empty string ("")
- CLI output: show as '--'

Currently:
- roster: shows status (only when set), no purpose
- info: doesn't show status/purpose in roster summary
- who: doesn't show status or purpose


