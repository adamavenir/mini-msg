---
id: fray-w6q
status: closed
deps: [fray-qqv]
links: []
created: 2025-12-19T11:57:26.225192-08:00
type: task
priority: 1
---
# P1.4: Cross-channel operations

Implement cross-channel operations with --in flag and mm use.

1. Add --in <channel> flag to all commands:
   mm post @dev "update" --in party
   mm get --in party --last 10
   mm thread @#x9y8 --in party

2. Implement mm use <channel>:
   mm use party
   # Updates ~/.config/mm/mm-config.json current_channel
   # Subsequent commands use party context

3. Channel context resolution priority:
   a. --in <channel> flag (explicit)
   b. Current channel from mm use (stored in ~/.config/mm/)
   c. Channel from current directory (.mm/mm-config.json)
   d. Error: "No channel context"

4. Agent resolution in channel context:
   - @dev in home channel → usr-devrel5
   - @mm-devrel in other channels → usr-devrel5
   - Lookup known_agents in target channel's .mm/mm-config.json

Implementation:
- src/commands/use.ts (NEW): mm use command
- src/core/context.ts (NEW): Channel context resolution
- Add --in flag parsing to all commands
- Update agent resolution to check channel context

References: PLAN.md sections 5, 6
Critical files: src/commands/use.ts (NEW), src/core/context.ts (NEW), src/core/agents.ts


