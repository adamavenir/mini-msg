---
name: hand
description: Hot handoff - end session with immediate continuation
allowed-tools: Bash, Read, Edit, TodoWrite
---

# Hot Handoff

**Use /hand when work is explicitly continuing NOW.** You know exactly what the next session should do. This is a baton pass mid-race.

Your name for this session: **$ARGUMENTS**

## Principles

- **Explicit continuation** - you KNOW the specific next steps, not just ideas
- **Preserve working context** - don't condense, the next session needs the raw details
- **Ensure all balls land somewhere** - tickets for discovered work, notes for context

## Handoff Sequence

### 1. Quick Standup

Post a brief "handing off" message to the room:

```bash
fray post "Handing off: <one-line what I was doing and where I stopped>" --as $ARGUMENTS
```

### 2. Review Session Notes

Check notes you wrote during this session:
```bash
fray get meta/$ARGUMENTS/notes
```

Ensure all "balls in the air" from your notes land somewhere:
- Work items → create tickets
- Key context → keep in handoff
- Ephemeral scratch → can be deleted if captured elsewhere

### 3. Capture Discovered Work

Create tickets for anything identified but not done:

```bash
tk create "..." --type task
# Add --label idea if it was a floating idea
```

### 4. Update Your Handoff Note

Check existing notes:
```bash
fray get meta/$ARGUMENTS/notes
```

If a handoff note exists, update it with current state:
```bash
fray edit <msg-id> "# Handoff

## In Progress
<what you were actively working on - be specific>

## Next Steps (EXPLICIT)
<what the fresh context should do FIRST - this is not a suggestion, it's the assignment>

## Key Context
<anything they need to understand current state>

## Open Questions
<unresolved issues, things needing discussion>
" --as $ARGUMENTS -m "hot handoff"
```

If no handoff exists, create one:
```bash
fray post meta/$ARGUMENTS/notes "# Handoff
..." --as $ARGUMENTS
```

**Don't condense** - the fresh context needs details, not summaries.

### 5. Commit Code

```bash
git status
fray claims  # check what others are working on
```

Commit your work. You MUST commit but ONLY commit your own work.

### 6. Update Changelog

If you completed user-facing changes (features, fixes, behavior changes), add them to CHANGELOG.md under the latest unreleased version.

### 7. Clear Claims

```bash
fray claims @$ARGUMENTS
fray clear @$ARGUMENTS
```

### 8. Hand Off

```bash
fray brb $ARGUMENTS "handing off to fresh context"
```

This tells the daemon to end your session and immediately spawn a fresh one.

## When to Use /hand vs /land

| Scenario | Use |
|----------|-----|
| Know EXACTLY what's next | **/hand** |
| Have IDEAS about what's next | /land |
| Context getting low, mid-task | **/hand** |
| Task complete, wrapping up | /land |
| Explicit continuation | **/hand** |
| Unknown when returning | /land |

## Key Difference from /land

The critical distinction: **/hand knows the explicit next work** (directly passing it off), while /land has **ideas/suggestions** about potential future work (which may become stale).

Both /hand and /land:
- Commit code
- Update changelog
- Create tickets for discovered work
- Update handoff notes

But /hand frames "Next Steps" as an **assignment**, while /land frames them as **suggestions**.
