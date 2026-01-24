---
id: fray-xfq
status: closed
deps: [fray-3vp]
links: []
created: 2025-12-19T11:57:25.648393-08:00
type: task
priority: 1
---
# P1.2: Chat reply-to with @#id

Implement chat reply-to with prefix matching resolution.

Parse @#prefix from input:
- Minimum 2 chars: @#a1, @#ab, @#a1b2
- Full GUID works: @#msg-a1b2c3d4
- Extract, resolve to GUID, strip from body

Resolution algorithm (src/core/guid-resolver.ts):
1. Normalize: @#a1b2 → msg-a1b2 (add msg- prefix if missing)
2. Query: SELECT guid FROM mm_messages WHERE guid LIKE 'msg-a1b2%' AND channel_id = ? ORDER BY created_at DESC LIMIT 5
3. If 0 matches: Error "No message found matching @#a1b2"
4. If 1 match: Use it
5. If 2+ matches: Error "Ambiguous reference @#a1b2. Did you mean:\n  @#a1b2c3 (2m ago): preview...\n  @#a1b2x9 (5m ago): preview..."

Scoping:
- Default: Current channel
- Full GUID (msg-xxx): Works globally

Implementation:
- src/chat/input.ts: Parse @#prefix, call resolver
- src/core/guid-resolver.ts (NEW): Resolution logic
- Strip @#prefix from body before storing
- Set reply_to field with full GUID

References: PLAN.md section 8 (Prefix resolution)
Critical files: src/chat/input.ts, src/core/guid-resolver.ts (NEW)


