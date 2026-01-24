---
id: fray-8jo
status: closed
deps: [fray-bml]
links: []
created: 2025-12-04T10:05:18.073351-08:00
type: task
priority: 1
parent: fray-dk4
---
# Implement timestamp utilities

Create src/core/time.ts with timestamp formatting utilities.

## Functions to Implement

```typescript
// src/core/time.ts

/**
 * Format unix timestamp as relative time.
 * @example formatRelative(now - 120) -> "2m ago"
 * @example formatRelative(now - 3600) -> "1h ago"
 * @example formatRelative(now - 86400) -> "1d ago"
 */
export function formatRelative(ts: number): string;

/**
 * Check if an agent is stale based on last seen time.
 * @param lastSeen - unix timestamp of last activity
 * @param staleHours - hours of inactivity threshold
 */
export function isStale(lastSeen: number, staleHours: number): boolean;

/**
 * Get current unix timestamp.
 */
export function now(): number;
```

## Relative Time Formatting Rules
| Duration | Format |
|----------|--------|
| < 60s | "Xs ago" (e.g., "30s ago") |
| < 60m | "Xm ago" (e.g., "5m ago") |
| < 24h | "Xh ago" (e.g., "2h ago") |
| < 7d | "Xd ago" (e.g., "3d ago") |
| >= 7d | "Xw ago" (e.g., "2w ago") |

Special case:
- "just now" for < 5 seconds (optional, can use "0s ago")

## Implementation Notes
- Use Math.floor for all divisions
- Unix timestamps are in seconds (not milliseconds)
- isStale: lastSeen + (staleHours * 3600) < now()

## Files
- src/core/time.ts

## Acceptance Criteria
- Relative time is human-readable
- Stale detection works correctly
- Unit tests for various time ranges
- Edge cases: exactly on boundary values


