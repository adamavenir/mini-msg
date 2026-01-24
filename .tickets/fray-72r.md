---
id: fray-72r
status: closed
deps: []
links: []
created: 2025-12-04T14:03:26.586925-08:00
type: bug
priority: 0
---
# Fix @mention shorthand bypassing Commander parsing

## Problem
The @mention shorthand (`bdm @alice`) bypasses Commander entirely (src/cli.ts:39-48).

```typescript
const args = process.argv.slice(2);
if (args.length > 0 && args[0].startsWith('@')) {
  mentionsAction(args[0], {...}, program);
  return;  // Skips program.parse()!
}
```

This means:
- `--project` flag is ignored → queries wrong database
- `--json` flag is ignored → loses JSON output
- No Commander validation happens

## Example
`bdm @alice --project api` silently queries the local project instead of the linked `api` project.

## Fix Options
1. Route mentions through a real Commander command (e.g., `bdm mentions @alice`)
2. Parse global options before dispatching to mentionsAction
3. Remove the shorthand entirely (breaking change)

Option 2 is likely best: manually parse --project and --json from args before calling mentionsAction.

## Files
- src/cli.ts:39-48

## Acceptance Criteria
- `bdm @alice --project api` queries the api project
- `bdm @alice --json` outputs JSON
- Existing `bdm @alice` behavior preserved


