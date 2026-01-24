---
id: fray-b56
status: closed
deps: []
links: []
created: 2025-12-21T14:03:59.749902-08:00
type: feature
priority: 2
---
# Add reactions for short replies (<20 char)

Reactions are 1-20 char responses (including emoji) that attach to messages rather than being full messages.

Syntax: #id 👍 or #id let's do it

Storage:
- Messages have a reactions object
- JSON groups users by reaction text:
  {"reactions": {"👍": ["alice", "bob"], "let's do it": ["charlie"]}}

Display in mentions:
- Grouped: '<user>, <user> reacted "👍" to "#msg-id Some truncated..."'

Live chat behavior:
- If reaction arrives after chat opened: append inline italic event + amend original message
- Event format: '<user> reacted "x" to "#msgid Some message..."' (no byline, just event)

Auto-detection:
- All replies <20 chars become reactions by default

Visual feedback in input:
- Text color yellow when <20 chars (reaction mode)
- Normal color when >=20 chars (reply mode)

Removal:
- Reactions can be removed but not edited
- Syntax: mm unreact #msg-id (removes your reaction)

Allows informal polls and quick responses without cluttering chat.


