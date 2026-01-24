---
id: fray-xpd
status: closed
deps: [fray-6cj, fray-1wj]
links: []
created: 2025-12-04T09:59:32.649203-08:00
type: task
priority: 1
parent: fray-08v
---
# Implement agent query functions

Create query functions for agent CRUD operations in src/db/queries.ts.

## Functions to Implement

```typescript
// src/db/queries.ts (agent functions)

import type { Agent } from '../types.js';
import type Database from 'better-sqlite3';

/**
 * Get agent by exact ID.
 * @returns Agent or undefined if not found
 */
export function getAgent(db: Database, agentId: string): Agent | undefined;

/**
 * Get agents matching a prefix.
 * Used for @mention resolution and "bdm who" command.
 * @example getAgentsByPrefix(db, "alice") -> [alice.1, alice.419, alice.frontend.3]
 * @example getAgentsByPrefix(db, "alice.frontend") -> [alice.frontend.1, alice.frontend.3]
 */
export function getAgentsByPrefix(db: Database, prefix: string): Agent[];

/**
 * Create new agent.
 * @throws if agent_id already exists
 */
export function createAgent(db: Database, agent: Omit<Agent, 'left_at'>): void;

/**
 * Update agent fields.
 * Only updates provided fields.
 */
export function updateAgent(
  db: Database, 
  agentId: string, 
  updates: Partial<Pick<Agent, 'goal' | 'bio' | 'last_seen' | 'left_at'>>
): void;

/**
 * Get active agents (not left, not stale).
 * @param staleHours - hours of inactivity before considered stale
 */
export function getActiveAgents(db: Database, staleHours: number): Agent[];

/**
 * Get all agents including stale and left.
 */
export function getAllAgents(db: Database): Agent[];

/**
 * Get highest version number for a base name.
 * Used by "bdm new" to auto-increment.
 * @example getMaxVersion(db, "alice") -> 419 (if alice.419 exists)
 * @returns 0 if no agents with this base exist
 * 
 * Implementation: Query for agent IDs matching pattern "base.N" where N is numeric.
 * SQL: SELECT agent_id FROM bdm_agents WHERE agent_id GLOB 'base.[0-9]*'
 * Then parse out the numeric suffix and return max.
 */
export function getMaxVersion(db: Database, base: string): number;
```

## Implementation Notes
- Use prepared statements for better performance
- Active = left_at IS NULL AND last_seen > (now - stale_hours * 3600)
- Prefix matching for getAgentsByPrefix: agent_id = prefix OR agent_id LIKE 'prefix.%'
- For getMaxVersion, use GLOB pattern to match base.N format, then parse results in JS

## SQL Examples

```sql
-- getActiveAgents
SELECT * FROM bdm_agents 
WHERE left_at IS NULL 
  AND last_seen > (strftime('%s', 'now') - ? * 3600)

-- getAgentsByPrefix (prefix = "alice")
SELECT * FROM bdm_agents
WHERE agent_id = 'alice' OR agent_id LIKE 'alice.%'

-- getMaxVersion (base = "alice")  
SELECT agent_id FROM bdm_agents
WHERE agent_id GLOB 'alice.[0-9]*'
-- Then in JS: parse each to extract number, return max
```

## Files
- src/db/queries.ts (add agent functions)

## Acceptance Criteria
- All functions work with real SQLite database
- Prepared statements are used
- Unit tests cover all functions
- Edge cases: empty results, prefix matching, getMaxVersion with no matches


