---
id: fray-bsj
status: closed
deps: []
links: []
created: 2025-12-04T14:03:48.763263-08:00
type: task
priority: 2
---
# Respect NO_COLOR env var in mention highlighting

## Problem
`highlightMentions()` always emits ANSI color codes (src/core/mentions.ts:50-55):

```typescript
export function highlightMentions(body: string): string {
  return body.replace(MENTION_REGEX, '\x1b[36m@\$1\x1b[0m');
}
```

Should respect the `NO_COLOR` environment variable standard (https://no-color.org/).

## Fix
```typescript
export function highlightMentions(body: string): string {
  if (process.env.NO_COLOR) {
    return body;  // No highlighting
  }
  return body.replace(MENTION_REGEX, '\x1b[36m@\$1\x1b[0m');
}
```

Also consider:
- Skipping colors when stdout is not a TTY (`!process.stdout.isTTY`)
- Skipping colors in JSON mode (caller responsibility)

## Files
- src/core/mentions.ts:50-55

## Acceptance Criteria
- NO_COLOR=1 bdm ... produces no ANSI codes
- Normal operation still has colored mentions


