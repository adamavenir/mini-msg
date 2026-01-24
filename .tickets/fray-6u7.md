---
id: fray-6u7
status: closed
deps: [fray-bml]
links: []
created: 2025-12-04T10:04:12.3668-08:00
type: task
priority: 1
parent: fray-dk4
---
# Implement agent ID utilities

Create src/core/agents.ts with agent ID parsing and formatting utilities.

## Agent ID Format
Format: `<role>[.<qualifier>]*.<session_number>`

Examples:
- `alice.1` - first alice session
- `alice.frontend.3` - third alice.frontend session  
- `pm.5.sub.2` - second subagent of pm session 5

Rules:
- Last segment is always numeric (version/session number)
- Base is everything before the last dot
- Role names are lowercase alphanumeric (no leading digits)

## Functions to Implement

```typescript
// src/core/agents.ts

import type { ParsedAgentId } from '../types.js';

/**
 * Parse an agent ID into components.
 * @throws if ID is invalid
 * @example parseAgentId("alice.419") -> { base: "alice", version: 419, full: "alice.419" }
 * @example parseAgentId("pm.3.sub.2") -> { base: "pm.3.sub", version: 2, full: "pm.3.sub.2" }
 */
export function parseAgentId(id: string): ParsedAgentId;

/**
 * Format base and version into agent ID.
 * @example formatAgentId("alice", 419) -> "alice.419"
 */
export function formatAgentId(base: string, version: number): string;

/**
 * Validate agent ID format.
 * @returns true if valid, false otherwise
 */
export function isValidAgentId(id: string): boolean;

/**
 * Validate base name format (role or role.qualifier).
 * @returns true if valid base name
 */
export function isValidBaseName(base: string): boolean;

/**
 * Normalize agent reference (strip leading @).
 * @example normalizeAgentRef("@alice.1") -> "alice.1"
 * @example normalizeAgentRef("alice.1") -> "alice.1"
 */
export function normalizeAgentRef(ref: string): string;

/**
 * Check if an agent ID matches a prefix.
 * Used for @mention resolution.
 * @example matchesPrefix("alice.419", "alice") -> true
 * @example matchesPrefix("alice.frontend.3", "alice") -> true
 * @example matchesPrefix("bob.1", "alice") -> false
 */
export function matchesPrefix(agentId: string, prefix: string): boolean;
```

## Validation Rules
- Base: lowercase letters, numbers, dots (no leading digit in any segment)
- Version: positive integer
- Full ID: base + "." + version
- No empty segments (e.g., "alice..1" invalid)

## Files
- src/core/agents.ts

## Acceptance Criteria
- Parsing extracts base and version correctly
- Validation catches all invalid formats
- Prefix matching works for all cases
- Comprehensive unit tests


