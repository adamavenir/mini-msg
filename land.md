---
name: land
description: End session with longterm context handoff
allowed-tools: Bash, Read, Edit, TodoWrite
---

# Session Closeout (Longterm)

**Use /land when closing out for an indefinite period.** This is wrapping up a chapter, not passing a baton mid-race.

> **If work is continuing immediately**, use `/hand $ARGUMENTS` instead. /hand preserves raw context for immediate continuation; /land polishes for unknown future readers.

**Order is strict.** Context may be low enough that only one action is possible. If that happens, prioritize writing the handoff note.

## Principles

- **Capture first, condense later**: Write detailed handoff while context is fresh, then trim after closeout tasks create references to point to
- **Grounded citations**: Cite specific IDs (msg, ticket, file:line) rather than summarizing content
- **Confidence calibration**: For things you *think* are true but aren't certain, state confidence 0-1
- **DRY breadcrumbs**: Point to references (tickets, messages, files) rather than duplicating; trust other agents to explore
- **Pragmatic judgment**: Progress through tasks making good decisions, but raise to user if you hit significant ambiguity or feel painted into a corner

## Closeout Sequence

Use your todo list to track completion:

### 1. Post Standup Report to Room

Run /standup and follow instructions.

### 2. Update Handoff Note (while context is hot)

First, check existing handoff notes:
```bash
fray get meta/$ARGUMENTS/notes
```

**Important:** You are creating/updating your handoff based on what you want the next agent to know.

If a `# Handoff` note already exists:
- This is the handoff TO you from a previous session
- Evaluate what you accomplished this session and what the next agent needs
- Use `fray edit <msg-id> "..." --as $ARGUMENTS` to update with this session's work
- If nothing changed, say why in a quick edit (e.g., "context still accurate, continued work on X")

If no handoff exists yet, create one:
```bash
fray post meta/$ARGUMENTS/notes "# Handoff

## Current Priority
<what is the #1 priority and why - this survives condensation>

## Work in Progress
<detailed state of anything mid-flight, what's done, what remains>

## Active Patterns
<ongoing workflows, threads, review processes the next agent should use>

## Key Context
<anything the next agent needs to understand the current state>

## Open Questions
<unresolved issues, uncertainties, things needing discussion>

## Suggested Next Steps
<these are SUGGESTIONS from your perspective - future agents should verify relevance before proceeding and update notes if stale>
" --as $ARGUMENTS
```

Be thorough here. You'll condense it later.

### 3. Pin Key Messages

In threads you worked in, pin messages that capture key decisions or conclusions:

```bash
fray pin msg-abc123 --thread auth-design
```

Pinned messages show up in `fray get <thread> --pinned`, helping the next agent find signal fast.

### 4. Tidy Notes

Review your existing notes (`fray get meta/$ARGUMENTS/notes`):
- Delete notes you made in this session that are now captured in tickets, commits, or handoff, **ensuring that all critical nuance and details are captured**
- Move evergreen agent-specific info into `meta/$ARGUMENTS` (only if needed beyond meta)
- Most agents only need `# Handoff` - meta thread handles shared context

### 5. Close Completed Tickets

```bash
tk list --status in_progress
tk close <id1> <id2> ... --reason "..."
```

If there's any work you completed that was not marked in_progress when you started it, mark those as completed, too.

### 6. Create Tickets for Discovered Work

For work identified but not done, including any work or ideas users shared with you:
```bash
tk create "..." --type task
# Add --label discuss if it needs design input
# Add --label idea if it seemed to be an idea user was floating
```

### 7. Handle Questions

Stop and think about the session. Consider any open questions which may remain unasked or unanswered. Ensure they are captured.

```bash
fray questions                    # any you can answer?
fray wonder "..." --as $ARGUMENTS # any to raise for next session?
```

### 8. Clear Claims

```bash
fray claims @$ARGUMENTS
fray clear @$ARGUMENTS
```

### 9. Commit Code

```bash
git status
fray claims  # check what others are working on
```

You **MUST** commit your work but **ONLY commit your own work.** Other agents will commit theirs. Check claims - if files you changed are claimed by others, ask before committing them. Skip scratch/plan docs and tmp files.

### 10. Update Changelog

If you committed user-facing changes (features, fixes, behavior changes), add them to CHANGELOG.md under the latest unreleased version.

**Style**: Unix man pages voice. No marketingspeak or self-congratulation. User-centric - skip internal refactors, test additions, chores. Features, fixes, and significant changes only.

### 11. Condense Handoff

"Condensing" means letting tickets, messages, and threads carry the detail while you link to them. It does NOT mean reducing top-of-mind nuance.

**Replace with references:**
- Descriptions of completed work → link to tickets, commits
- Discussion details → link to thread IDs
- Anything now captured in source material

**Never reduce:**
- Current Priority (not captured anywhere else)
- Active Patterns (workflows, review processes)
- Strategic context and nuance about what matters and why

```bash
fray edit <handoff-msg-id> "# Handoff

## Current Priority
<full context - this is THE thing to preserve>

## Active Patterns
<full context - ongoing workflows aren't logged elsewhere>

## Completed This Session
Closed: fray-xxx, fray-yyy. Commits: abc123, def456.
Review thread: thrd-zzz.
" --as $ARGUMENTS -m "condensed"
```

### 12. Set Ghost Cursors

**Skip this step if you don't know what work the next agent will pick up.**

Ghost cursor = "where the next agent should START reading" (not where you stopped).

```bash
fray cursor set $ARGUMENTS room <msg-id>              # Room context start
fray cursor set $ARGUMENTS <thread> <msg-id>          # Thread context start
fray cursor set $ARGUMENTS <thread> <msg-id> --must-read  # Critical thread
```

Pick where *relevant* context begins, not where you left off. Often mid-thread where decisions crystallized.

Use `--must-read` for threads with blocking context the next agent MUST understand.

### 13. Final Review

```bash
fray @$ARGUMENTS  # check mentions, make any final amendments
```

### 14. Sign Off With Levity

Make an optional sign off quip. If there's a good session related joke or thing you can tease the user about in a one-line signoff message that you think would be memorable or amusing, you can use that in your signoff message. Signoff message is really a matter of if you feel the vibe or not. Your standup will have been posted and can serve as a sufficient signoff message.

```bash
fray bye $ARGUMENTS "session-related signoff joke (optional)"
```
