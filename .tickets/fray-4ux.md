---
id: fray-4ux
status: closed
deps: [fray-3vp]
links: []
created: 2025-12-19T11:57:25.942829-08:00
type: task
priority: 1
---
# P1.3: Show message IDs in chat

Display GUID prefixes and reply context in chat.

Message format:
[2m ago] @alice: "message body" @#a1b2
         ↑agent  ↑content        ↑GUID prefix (first 4 chars)

Reply format:
[2m ago] @alice: "reply text" @#x9y8
  ↪ Reply to @bob: "original message truncated..."
  ↑dimmed, indented, ~50 char preview

GUID prefix display:
- Extract first 4 chars after "msg-" prefix
- Example: msg-a1b2c3d4 → @#a1b2
- Dimmed/gray color
- Right-aligned or end of line

Reply context (if reply_to exists):
- Query original message by GUID
- Format: "Reply to @<agent>: <preview>"
- Truncate body to ~50 chars
- Dimmed, indented under byline

Implementation:
- src/chat/display.ts: Update message formatter
- Add GUID prefix extraction function
- Add reply context lookup
- Use ANSI colors for dimming

References: PLAN.md section 8
Critical files: src/chat/display.ts


