---
id: fray-hj9
status: closed
deps: [fray-w6q, fray-3iq]
links: []
created: 2025-12-19T13:31:03.020705-08:00
type: task
priority: 1
---
# P1.8: Tests for channel system and agents

Add tests for channel operations and agent discovery.

Test files to create/update:

1. tests/channels.test.ts (NEW)
   - Channel GUID generation
   - .mm/mm-config.json creation
   - Global config registration
   - Channel context resolution (--in flag, mm use)
   - Cross-channel message references

2. tests/agent-discovery.test.ts (NEW)
   - Progressive discovery flow
   - Reuse existing agent GUID
   - Create new agent on collision
   - known_agents updates
   - Auto-nick generation (home channel)

3. tests/prefix-resolution.test.ts (NEW)
   - Unique match works
   - Ambiguous match errors with suggestions
   - Channel scoping (default vs full GUID)
   - Minimum 2-char prefix
   - Case sensitivity

4. tests/time-queries.test.ts (NEW)
   - Parse relative time (1h, 2d)
   - Parse absolute time (yesterday, today)
   - Parse GUID references (@#a1b2)
   - --since, --before, --from, --to flags
   - Invalid input handling

5. tests/integration.test.ts (UPDATE)
   - End-to-end GUID flow
   - Multi-channel coordination
   - Agent migration scenarios
   - Prune with guardrails

Coverage goals:
- Channel operations: 95%
- Agent discovery: 95%
- Prefix resolution: 100%
- Time queries: 90%

Run with: npm test

References: Existing integration tests
Critical files: tests/channels.test.ts (NEW), tests/agent-discovery.test.ts (NEW), tests/prefix-resolution.test.ts (NEW)


