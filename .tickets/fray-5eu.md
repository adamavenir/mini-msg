---
id: fray-5eu
status: closed
deps: []
links: []
created: 2025-12-04T14:03:48.863915-08:00
type: bug
priority: 2
---
# Fix error message claiming dashes are allowed in agent names

## Problem
Error message in new.ts claims dashes are allowed, but the validator rejects them:

```typescript
// src/commands/new.ts:18
throw new Error(\`Invalid agent base name: \${name}. Must be alphanumeric with dots/dashes.\`);

// src/core/agents.ts:95 - actual regex
const roleRegex = /^[a-z][a-z0-9]*$/;  // No dashes!
```

## Options
1. Fix the error message to match reality: "Must start with lowercase letter, contain only letters, numbers, and dots"
2. Update the regex to actually allow dashes (may have implications)

Option 1 is safer - just align the message with the actual validation.

## Files
- src/commands/new.ts:18

## Acceptance Criteria
- Error message accurately describes what's allowed
- Or dashes are actually allowed (if that's the intent)


