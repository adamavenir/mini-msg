---
id: fray-ayw
status: open
deps: []
links: []
created: 2025-12-09T06:12:03.002401-08:00
type: feature
priority: 2
---
# Replace --last suggestion with --since, add token limit

The suggestion at the end of `bdm get` encourages agents to use `--last 50` which immediately consumes all their tokens.

## Current Behavior

At the end of `bdm get alice.1`, we show:
```
More: bdm get --last 50 | bdm @alice --all | bdm get --since <id>
```

Agents see `--last 50` and immediately run it, eating their context.

## Desired Behavior

1. Remove `--last 50` suggestion entirely
2. Suggest `bdm get --since <id>` instead
3. Add token protection: error if requesting more than 10 messages back without explicit flag

**New suggestion line:**
```
More: bdm @alice --all | bdm get --since <id>
```

**Token limit enforcement:**
```bash
bdm get --last 50        # ERROR: "Requesting 50 messages. Use --bypass-token-limit to proceed."
bdm get --since 100      # If >10 messages: ERROR with same message
bdm get --last 50 --bypass-token-limit   # OK, proceeds
```

## Implementation

- Remove `--last 50` from suggestion in get.ts
- Add validation in get.ts and mentions.ts:
  - Count how many messages would be returned
  - If >10, error unless `--bypass-token-limit` flag is present
- Add `--bypass-token-limit` flag to relevant commands

## Files

- src/commands/get.ts - Update suggestion, add token limit check
- src/commands/mentions.ts - Add token limit check


