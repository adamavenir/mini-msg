---
id: fray-92p
status: closed
deps: [fray-6u7]
links: []
created: 2025-12-04T10:04:41.987627-08:00
type: task
priority: 1
parent: fray-dk4
---
# Implement mention parsing utilities

Create src/core/mentions.ts with @mention extraction and matching utilities.

## Mention Format
- Mentions start with @ followed by agent ID or prefix
- @all is special broadcast mention
- Can mention full IDs (@alice.419) or prefixes (@alice)
- Examples: @alice, @alice.419, @pm.3.sub, @all

## Functions to Implement

```typescript
// src/core/mentions.ts

/**
 * Extract all @mentions from message body.
 * Returns mention targets without @ prefix.
 * @example extractMentions("hey @alice.1 and @bob.3") -> ["alice.1", "bob.3"]
 * @example extractMentions("@all heads up") -> ["all"]
 * @example extractMentions("@alice check this") -> ["alice"]
 */
export function extractMentions(body: string): string[];

/**
 * Check if an agent ID matches a mention prefix.
 * Used to determine if a message is relevant to an agent.
 * @example matchesMention("alice.419", "alice") -> true (prefix match)
 * @example matchesMention("alice.419", "alice.419") -> true (exact match)
 * @example matchesMention("alice.frontend.3", "alice") -> true (prefix match)
 * @example matchesMention("bob.1", "alice") -> false
 */
export function matchesMention(agentId: string, mentionPrefix: string): boolean;

/**
 * Check if a mention is the broadcast mention.
 */
export function isAllMention(mention: string): boolean;

/**
 * Highlight mentions in message body for display (optional - can defer to Phase 5).
 * @example highlightMentions("hey @alice") -> "hey \x1b[36m@alice\x1b[0m"
 */
export function highlightMentions(body: string): string;
```

## Regex Pattern for Mentions
```typescript
// Match @ followed by:
// - "all" (broadcast)
// - OR valid agent ID/prefix: starts with letter, then letters/numbers/dots
// The pattern allows both full IDs (alice.419) and prefixes (alice)
const MENTION_REGEX = /@(all|[a-z][a-z0-9]*(?:\.[a-z0-9]+)*)/g;
```

Note: This regex matches both `@alice` (prefix) and `@alice.419` (full ID). The `.419` part is captured because `[a-z0-9]+` allows digits.

## matchesMention Logic
```typescript
function matchesMention(agentId: string, mentionPrefix: string): boolean {
  // Exact match
  if (agentId === mentionPrefix) return true;
  // Prefix match: agentId starts with prefix followed by dot
  if (agentId.startsWith(mentionPrefix + '.')) return true;
  return false;
}
```

## Edge Cases
- Multiple mentions in one message: "@alice @bob both of you"
- Mentions at start/end of message
- Mentions with punctuation after: "@alice, please" → extracts "alice"
- @all mixed with specific mentions: "@all and especially @alice"
- Email-like strings should NOT match: "contact@alice.com" should not extract "alice.com"
  - The regex handles this because it requires @ at word boundary

## Files
- src/core/mentions.ts

## Acceptance Criteria
- Extracts all valid mentions from message body
- Does not extract from email addresses
- Handles punctuation correctly
- Prefix matching works per spec
- Unit tests cover all patterns including edge cases


