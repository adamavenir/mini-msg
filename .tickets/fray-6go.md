---
id: fray-6go
status: closed
deps: []
links: []
created: 2025-12-07T17:11:41.405464-08:00
type: feature
priority: 2
---
# Assign chat colors by recency, not hash

In bdm chat mode, assign colors to agents based on conversation recency rather than hash-based assignment.

## Problem

Currently colors are assigned by hashing the agent base name modulo 6 color sets. With only 6 colors, hash collisions are common - two active agents might get the same color even though there are unused colors available.

## Desired Behavior

When entering chat mode, assign colors to agents based on who has posted recently:

1. Get list of agents who have posted in recent history (e.g., last 50 messages)
2. Sort by most recent activity
3. Assign colors sequentially: first agent gets color 0, second gets color 1, etc.
4. This guarantees the first 6 active agents get unique colors
5. Less active/older agents might share colors, but they're not in the active conversation

## Implementation

**Approach**: Build a color map at chat session start based on recent messages

```typescript
// In chat session initialization
function buildColorMap(db: Database, lookbackLimit: number = 50): Map<string, number> {
  const messages = getMessages(db, { limit: lookbackLimit });
  const agentBases = new Map<string, number>(); // base -> last seen timestamp
  
  for (const msg of messages) {
    if (msg.type === 'agent') {
      const base = parseAgentId(msg.from_agent).base;
      const ts = msg.ts;
      if (!agentBases.has(base) || agentBases.get(base)! < ts) {
        agentBases.set(base, ts);
      }
    }
  }
  
  // Sort by most recent
  const sorted = Array.from(agentBases.entries())
    .sort((a, b) => b[1] - a[1]);
  
  // Assign colors sequentially
  const colorMap = new Map<string, number>();
  sorted.forEach(([base, _], idx) => {
    colorMap.set(base, idx % COLOR_PAIRS.length);
  });
  
  return colorMap;
}
```

## Files to Modify

- `src/chat/display.ts` - Accept colorMap in constructor, use it in getAgentColor calls
- `src/commands/chat.ts` - Build colorMap on session start, pass to display
- `src/commands/format.ts` - Add `getAgentColorWithMap()` variant that takes optional colorMap

## Fallback

For agents not in the recent history, fall back to hash-based assignment so they still get a color.

## Benefits

- Active conversation participants are guaranteed visually distinct
- No more confusing collisions between agents who are talking
- Still works for new agents who join mid-session (hash fallback)
- Improves readability during active multi-agent conversations


