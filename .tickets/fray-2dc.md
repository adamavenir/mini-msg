---
id: fray-2dc
status: closed
deps: [fray-2vy, fray-dgt]
links: []
created: 2025-12-04T10:09:14.178017-08:00
type: task
priority: 1
parent: fray-yh4
---
# Implement @mentions query command

Implement the @mentions shorthand for querying messages mentioning an agent.

## Usage

```bash
bdm @alice                       # last 5 messages mentioning @alice*
bdm @alice.419                   # messages mentioning exactly @alice.419*
bdm @alice --last 20             # last 20 mentions
bdm @alice --since 42            # mentions after ID 42
```

## Detection
When the first positional argument starts with @, treat as mentions query:
- Strip @ prefix to get agent prefix
- Query messages where any mention matches prefix

## Implementation

1. Detect @-prefixed first argument in CLI
2. Extract prefix: "@alice" → "alice"
3. Call getMessagesWithMention(db, prefix, options)
4. Format same as regular message display

## Mention Matching (in database query)
For message to match "@alice":
- Any mention in mentions array equals "alice", OR
- Any mention starts with "alice."
- OR any mention is "all" (broadcasts included)

SQL (using json_each):
```sql
SELECT DISTINCT m.* FROM bdm_messages m, json_each(m.mentions) j
WHERE j.value = 'all' 
   OR j.value = ?  -- exact match
   OR j.value LIKE ? || '.%'  -- prefix match
ORDER BY m.id DESC
LIMIT ?
```

## Default Limit
- Default: 5 messages (vs 20 for room history)
- Rationale: mentions are more targeted, smaller default

## Output
Same format as regular messages:
```
[43] bob.3 (1m ago): @alice.419 got it, thanks
[44] pm.5 (30s ago): @all standup in 5
```

## Files
- src/commands/mentions.ts
- src/cli.ts (detect @ prefix and route)

## Acceptance Criteria
- @agent shows messages mentioning that agent
- Prefix matching works (@ alice matches @alice.419)
- @all broadcasts are included
- --last and --since options work
- Default limit is 5


