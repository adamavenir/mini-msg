---
id: fray-g5e
status: closed
deps: [fray-2vy, fray-xpd, fray-6u7]
links: []
created: 2025-12-04T10:08:21.729327-08:00
type: task
priority: 1
parent: fray-yh4
---
# Implement lifecycle commands (new, hi, bye)

Create lifecycle commands for agent session management.

## Commands

### bdm new <name>
Create a new agent session with auto-incremented version.

```bash
bdm new alice                    # → alice.1 (or alice.N+1)
bdm new alice --goal "..."       # with goal
bdm new alice --bio "..."        # with bio
```

**Implementation:**
1. Parse and validate base name
2. Query getMaxVersion(db, base) to find highest version
3. Create agent with version = max + 1
4. Set registered_at and last_seen to now()
5. Output: "Registered as alice.N"

### bdm hi <agent.N>
Resume a previous session (clear left_at, update last_seen).

```bash
bdm hi alice.419
```

**Implementation:**
1. Parse and validate full agent ID
2. Verify agent exists
3. Update: left_at = NULL, last_seen = now()
4. Output: "Welcome back, alice.419"

**Error cases:**
- Agent doesn't exist: "Unknown agent: alice.419"
- Agent never left: "alice.419 is already active"

### bdm bye <agent.N>
Leave the session (set left_at).

```bash
bdm bye alice.419
```

**Implementation:**
1. Parse and validate full agent ID
2. Verify agent exists and is active
3. Update: left_at = now()
4. Output: "Goodbye, alice.419"

**Error cases:**
- Agent doesn't exist: "Unknown agent: alice.419"
- Agent already left: "alice.419 has already left"

## Files
- src/commands/new.ts
- src/commands/hi.ts
- src/commands/bye.ts

## Acceptance Criteria
- new creates agent with correct version
- hi reactivates agent
- bye marks agent as left
- Clear error messages for all failure cases
- Integration test for full lifecycle


