---
id: fray-4ry
status: open
deps: [fray-6qu, fray-7mm]
links: []
created: 2025-12-19T13:31:03.458326-08:00
type: task
priority: 2
---
# P2.3: Tests for migration and edge cases

Add tests for migration path and edge cases.

Test files to create/update:

1. tests/migration.test.ts (NEW)
   - Detect old format correctly
   - Generate GUIDs for all existing data
   - Preserve message order
   - Create .mm/mm-config.json
   - Backup to .mm.bak/
   - Handle empty database
   - Handle corrupted old data

2. tests/prune.test.ts (NEW)
   - Guardrails: uncommitted changes
   - Guardrails: ahead/behind remote
   - Guardrails: no remote (offline OK)
   - Archive to history.jsonl
   - Keep last N messages
   - --all flag wipes history
   - GUID stability after prune

3. tests/multi-machine.test.ts (NEW)
   - Simulate git pull with new messages
   - SQLite rebuild from merged JSONL
   - Agent GUID conflicts
   - Channel ID stability
   - Known_agents merge scenarios

4. tests/chat.test.ts (UPDATE)
   - @#prefix parsing in input
   - Reply-to resolution
   - Display GUID prefixes
   - Reply context formatting
   - Ambiguous GUID errors

Edge cases:
- Empty database
- Malformed JSONL
- Missing config files
- Offline operation
- GUID collisions (simulate)
- Very long message IDs

Coverage goals:
- Migration: 90%
- Prune: 95%
- Edge cases: 80%

Run with: npm test

References: Existing integration tests
Critical files: tests/migration.test.ts (NEW), tests/prune.test.ts (NEW), tests/multi-machine.test.ts (NEW)


