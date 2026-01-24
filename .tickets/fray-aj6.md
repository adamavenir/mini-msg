---
id: fray-aj6
status: closed
deps: []
links: []
created: 2025-12-31T14:42:38.378412-08:00
type: bug
priority: 1
---
# fray get <agent> should default to unread-only

The watermark system (4258ebd) tracks read positions but `fray get <agent>` doesn't use it by default.

## Current Behavior
- `fray get <agent>` returns last N room messages regardless of watermark
- Agents waste tokens re-reading already-seen messages

## Proposed Behavior
- `fray get <agent>` → unread only (messages since agent's watermark)
- `fray get <agent> --last 10` → last 10 regardless of watermark (cap at 10-20)
- `fray get <agent> --since X --to Y` → explicit range for history pagination
- Remove or hide `--all` for agents (token waste)

## Additional: Chained Replies in Mentions
Replies to an agent's messages (via reply chain) should be included in the 'mentions' result, not just explicit @mentions. If someone replies to my question, I should see it even without an explicit @designer.

## Context
Discovered during design session - agents repeatedly calling `fray get designer --room 8` burning tokens on already-read content.

See msg-wor9q3ge for discussion, msg-kj4bo1tn for reply chain note.


