---
id: fray-9si
status: closed
deps: [fray-2vy, fray-92p, fray-dgt]
links: []
created: 2025-12-04T10:09:31.908202-08:00
type: task
priority: 1
parent: fray-yh4
---
# Implement post command

Implement the post command for sending messages.

## Usage

```bash
bdm post --as alice.419 "hello world"
bdm post --as alice.419 "@bob.3 check auth.py:47"
bdm post --as pm.5 "@all standup in 5"
```

## Implementation

1. Validate --as agent exists and is active
2. Extract mentions from message body
3. Create message with:
   - ts: now()
   - from_agent: agent ID from --as
   - body: message text
   - mentions: extracted mentions array
4. Update agent's last_seen to now()
5. Output: Show the posted message

## Validation

**Agent validation:**
- Agent must exist
- Agent must not have left_at set (or be resumed first)
- Error: "Agent alice.419 is not active. Use 'bdm hi alice.419' to resume."

**Message validation:**
- Body must not be empty
- Error: "Message cannot be empty"

## Side Effects
- Creates message in bdm_messages
- Updates agent's last_seen (heartbeat)

## Output

```
Posted:
[45] alice.419 (just now): hello world
```

Or minimal:
```
→ [45]
```

## JSON Output

```json
{
  "id": 45,
  "ts": 1234567890,
  "from_agent": "alice.419",
  "body": "hello world",
  "mentions": []
}
```

## Files
- src/commands/post.ts

## Acceptance Criteria
- Post creates message correctly
- Mentions are extracted and stored
- Agent last_seen is updated
- Validation rejects inactive agents
- Validation rejects empty messages


