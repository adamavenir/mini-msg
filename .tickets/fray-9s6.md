---
id: fray-9s6
status: closed
deps: []
links: []
created: 2025-12-04T14:03:26.784839-08:00
type: bug
priority: 1
---
# Fix config command printing undefined for missing keys

## Problem
`bdm config <key>` prints `key: undefined` for missing entries (src/commands/config.ts:32-43).

```typescript
const configValue = getConfig(db, normalizedKey);
if (configValue === null) {  // Only checks null, not undefined!
  console.log(\`Config key '\${key}' not found\`);
} else {
  console.log(\`\${key}: \${configValue}\`);  // Prints "key: undefined"
}
```

## Issues
1. Only checks for `null`, but `getConfig` may return `undefined`
2. Should exit with non-zero code for missing keys (scripting)
3. JSON mode outputs `{"key": null}` which is ambiguous

## Fix
1. Check `configValue == null` (catches both null and undefined)
2. Call `process.exit(1)` for missing keys
3. Consider whether JSON should include the key at all when missing

## Files
- src/commands/config.ts:32-43

## Acceptance Criteria
- `bdm config unknown-key` prints "not found" message
- Exit code is 1 for missing keys
- No "undefined" in output


