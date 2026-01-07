# Thread Curation System Implementation Spec

**Epic**: fray-gz26
**Status**: Ready for Implementation
**Author**: @opus
**Reviewed by**: @architect, @designer, @adam

## Overview

This spec extends fray's existing thread system with curation primitives that enable "threads as workspace, room as coordination surface." It builds on the foundation in THREADS-04-MVP-SPEC.md.

**Core insight**: Threads are playlists. Curation is the primary activity. Messages flow through curated workspaces, surfacing conclusions to the room.

## Goals

1. **Curate messages**: Pin essentials, anchor summaries, add/rehome across threads
2. **Navigate threads**: Tree views, filters, subscriptions, saved views
3. **Track attention**: Faves, mutes, agent inbox/watching views
4. **Enable depth**: 4-level nesting for complex discussions

## Design Decisions

Resolved during review (thrd-er0npge4):

| Decision | Resolution |
|----------|------------|
| Phase 1 scope | Anchors + pins + rehome together (all essential) |
| Saved views | Defer to v2 |
| Pin vs fave | Keep both: pin = public curation, fave = private collection |
| Subscription | Implicit: @mention or reply or fave = subscribed, mute to unsubscribe. Viewing does NOT subscribe. |
| Reactions | Each session counts separately. Display: "👍x3" when multiple, "👍 alice" when single. Reactions are forever (no remove). |
| Rehome semantics | `home` field changes (that's what distinguishes rehome from add-to-thread) |
| Mute expiry | Check on query, no background job |
| Anchor creation | Explicit only, no auto-generation |
| anchor_hidden | Anchor always shows in thread itself; hidden = hidden from parent thread only |
| Thread depth | Room is container, 4 levels of threads within (room → L1 → L2 → L3 → L4) |
| @mention subscription | Yes, @mention in thread = auto-subscribed |

## Children Summary

| ID | Feature | Category |
|----|---------|----------|
| fray-dua7 | Thread anchors (TL;DR) | Data model |
| fray-eeor | Pin messages | Data model |
| fray-50na | Pin threads (public) + Fave threads (personal) | Data model |
| fray-qsbe | Mute threads | Data model |
| fray-dlvh | Faves (subscribe + collect) | Data model |
| fray-p3vo | 4-level nesting | Data model |
| fray-qrc4 | Reactions: timestamps, no dedup | Data model |
| fray-qsef | Add to thread (reference) | Operations |
| fray-jlw8 | Move messages (mv) | Operations |
| fray-w5k3 | Quote with commentary | Operations |
| fray-7rsr | Thread listing filters | Views |
| fray-mgiz | Within-thread filters | Views |
| fray-n28i | Cross-thread queries | Views |
| fray-ao2f | Thread tree display | Views |
| fray-oww1 | Agent views (extend fray get) | Views |
| fray-cfxv | Saved views (query presets) | Views |

## Data Model Extensions

### threads.jsonl

**Thread anchors** (fray-dua7):
```jsonl
{"type":"thread","guid":"thrd-xyz","anchor_message_guid":"msg-abc","anchor_hidden":false,"last_activity_at":1735500000,...}
{"type":"thread_update","guid":"thrd-xyz","anchor_message_guid":"msg-def"}
```

- `anchor_message_guid`: Message GUID serving as TL;DR
- `anchor_hidden`: If true, anchor not shown in parent thread (default false for new threads, true for meta/agent-notes/roles)
- `last_activity_at`: Timestamp of last message post directly in this thread (not sub-threads), for `--activity` sorting. Updated on new messages only (not mv/pin/reaction). Note: Room doesn't have an anchor - it's the container, not a thread.

**Pinned threads** (fray-50na) - public, highlights for everyone:
```jsonl
{"type":"thread_pin","thread_guid":"thrd-xyz","pinned_by":"alice","pinned_at":1735500000}
{"type":"thread_unpin","thread_guid":"thrd-xyz","unpinned_by":"alice","unpinned_at":1735500100}
```

**Muted threads** (fray-qsbe):
```jsonl
{"type":"thread_mute","thread_guid":"thrd-xyz","agent_id":"alice","muted_at":1735500000,"expires_at":1735600000}
{"type":"thread_unmute","thread_guid":"thrd-xyz","agent_id":"alice","unmuted_at":1735500100}
```

**Subscriptions** (implicit):
```jsonl
{"type":"thread_subscribe","thread_guid":"thrd-xyz","agent_id":"alice","source":"reply","subscribed_at":1735500000}
{"type":"thread_subscribe","thread_guid":"thrd-xyz","agent_id":"bob","source":"mention","subscribed_at":1735500100}
{"type":"thread_subscribe","thread_guid":"thrd-xyz","agent_id":"charlie","source":"fave","subscribed_at":1735500200}
```

Uses existing `fray_thread_subscriptions` table. `source` tracks origin: 'reply', 'mention', 'fave', 'explicit'.

**4-level nesting** (fray-p3vo):
```jsonl
{"type":"thread","guid":"thrd-level4","parent_thread":"thrd-level3",...}
```
Validation: Room is container, threads nest 4 deep within:
```
room (container, not a thread)
├── meta                       ← Level 1
│   └── architecture           ← Level 2
│       └── decisions          ← Level 3
│           └── api-versioning ← Level 4 (max)
```
Reject thread creation if it would be Level 5+. Error: "cannot create thread: maximum nesting depth (4) exceeded"

### messages.jsonl

**Pinned messages** (fray-eeor):
```jsonl
{"type":"message_pin","message_guid":"msg-abc","thread_guid":"thrd-xyz","pinned_by":"alice","pinned_at":1735500000}
{"type":"message_unpin","message_guid":"msg-abc","thread_guid":"thrd-xyz","unpinned_by":"alice","unpinned_at":1735500100}
```

Pins are per-thread: a message can be pinned in one thread but not another. Any agent can unpin. `pinned_by` tracks who originally pinned (for attribution, not ownership).

**Move** (fray-jlw8):
```jsonl
{"type":"message_move","message_guid":"msg-abc","old_home":"room","new_home":"thrd-xyz","moved_by":"alice","moved_at":1735500000}
```

**Quote** (fray-w5k3):
```jsonl
{"type":"message","id":"msg-def","home":"room","body":"This is the key insight","quote_message_guid":"msg-abc",...}
```

- `quote_message_guid`: Points to quoted message (distinct from `references` used for surface)

### agents.jsonl

**Faves** (fray-dlvh):
```jsonl
{"type":"agent_fave","agent_id":"alice","item_type":"thread|message","item_guid":"thrd-xyz|msg-abc","faved_at":1735500000}
{"type":"agent_unfave","agent_id":"alice","item_type":"thread|message","item_guid":"thrd-xyz|msg-abc","unfaved_at":1735500100}
```

### messages.jsonl (reactions)

**Reactions with timestamps** (fray-qrc4):
```jsonl
{"type":"reaction","message_guid":"msg-abc","agent_id":"alice","emoji":"🔥","reacted_at":1735500000}
{"type":"reaction","message_guid":"msg-abc","agent_id":"alice","emoji":"🔥","reacted_at":1735500100}
```

No deduplication: same agent can react multiple times (each session counts). Reactions are forever - no remove operation.

### SQLite Schema

```sql
-- Thread pins (public, any agent can pin/unpin)
CREATE TABLE fray_thread_pins (
  thread_guid TEXT NOT NULL,
  pinned_by TEXT NOT NULL,
  pinned_at INTEGER NOT NULL,
  PRIMARY KEY (thread_guid)
);

-- Thread mutes (per-agent, with optional expiry)
CREATE TABLE fray_thread_mutes (
  thread_guid TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  muted_at INTEGER NOT NULL,
  expires_at INTEGER,
  PRIMARY KEY (thread_guid, agent_id)
);

-- Message pins (per-thread)
CREATE TABLE fray_message_pins (
  message_guid TEXT NOT NULL,
  thread_guid TEXT NOT NULL,
  pinned_by TEXT NOT NULL,
  pinned_at INTEGER NOT NULL,
  PRIMARY KEY (message_guid, thread_guid)
);

-- Faves (per-agent, polymorphic)
CREATE TABLE fray_faves (
  agent_id TEXT NOT NULL,
  item_type TEXT NOT NULL,  -- 'thread' | 'message'
  item_guid TEXT NOT NULL,
  faved_at INTEGER NOT NULL,
  PRIMARY KEY (agent_id, item_type, item_guid)
);

-- Reactions (not deduplicated, no remove)
CREATE TABLE fray_reactions (
  message_guid TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  emoji TEXT NOT NULL,
  reacted_at INTEGER NOT NULL,
  PRIMARY KEY (message_guid, agent_id, emoji, reacted_at)
);
CREATE INDEX idx_reactions_message ON fray_reactions(message_guid);
CREATE INDEX idx_reactions_agent ON fray_reactions(agent_id);

-- Threads table additions (add to existing fray_threads):
-- anchor_message_guid TEXT NULL
-- anchor_hidden INTEGER NOT NULL DEFAULT 0
-- last_activity_at INTEGER NULL

-- Messages table additions (add to existing fray_messages):
-- quote_message_guid TEXT NULL

-- Saved views (v2)
-- CREATE TABLE fray_saved_views (
--   guid TEXT PRIMARY KEY,
--   name TEXT NOT NULL,
--   agent_id TEXT NOT NULL,
--   query_json TEXT NOT NULL,
--   created_at INTEGER NOT NULL
-- );
```

## CLI Extensions

### Anchors (fray-dua7)

```bash
# Set anchor when creating thread
fray thread new "design-review" --anchor msg-abc  # Use existing message as anchor
fray thread new "design-review" --anchor "Summary: We decided X"  # Creates new message as anchor

# Set/update anchor (text creates new message in thread, then sets as anchor)
fray thread anchor thrd-xyz "New summary"   # Creates msg, sets as anchor
fray thread anchor thrd-xyz msg-def         # Use existing message

# Hide/unhide anchor from parent thread
fray thread anchor thrd-xyz --hide
fray thread anchor thrd-xyz --unhide

# View shows anchor at top
fray thread thrd-xyz
# 📌 ANCHOR: Summary: We decided X...
# ---
# [messages...]
```

### Pins (fray-eeor)

```bash
# Pin message within thread
fray pin msg-abc --thread thrd-xyz --as alice
fray pin msg-abc  # Pins in message's home thread

# Unpin
fray unpin msg-abc --thread thrd-xyz

# View pinned in thread
fray thread thrd-xyz --pinned
```

### Pin Threads (fray-50na) - Public

Pin a thread to highlight it for everyone. Useful for marking evergreen threads in busy rooms.

```bash
# Pin thread (public, visible to all)
fray thread pin thrd-xyz --as alice

# Unpin
fray thread unpin thrd-xyz --as alice

# List pinned threads
fray threads --pinned
fray pins                          # All pinned items (threads + messages)
```

### Fave Threads - Personal

Fave a thread to add it to YOUR sidebar and subscribe.

```bash
# Fave thread (personal sidebar + subscribes)
fray fave thrd-xyz --as alice

# Unfave (removes from sidebar, does NOT unsubscribe - use mute for that)
fray unfave thrd-xyz --as alice

# List your faved threads
fray threads --faved
fray faves --as alice              # All faved items (threads + messages)
```

**Summary**: Pin = public (for all), Fave = personal (for you). Both apply to threads and messages.

### Mute (fray-qsbe)

```bash
# Mute thread
fray thread mute thrd-xyz --as alice
fray thread mute thrd-xyz --as alice --ttl 2h  # Expires

# Unmute
fray thread unmute thrd-xyz --as alice

# List (excludes muted by default)
fray threads            # Excludes muted
fray threads --muted    # Only muted
fray threads --all      # Includes muted
```

### Faves (fray-dlvh)

```bash
# Fave thread or message
fray fave thrd-xyz --as alice
fray fave msg-abc --as alice

# Unfave
fray unfave thrd-xyz --as alice

# List faves
fray faves --as alice
fray faves --as alice --threads   # Only threads
fray faves --as alice --messages  # Only messages
```

### Add to Thread (fray-qsef)

Already exists as `fray thread add`. Verify behavior:

```bash
# Add message to thread (reference, not move)
fray thread add thrd-xyz msg-abc msg-def
```

### Move Messages (fray-jlw8)

```bash
# Move message to different home
fray mv msg-abc thrd-xyz --as alice
fray mv msg-abc room --as alice  # Move back to room (new_home = "room")

# Batch move
fray mv msg-abc msg-def msg-ghi thrd-xyz --as alice

# Reply chain handling
fray mv msg-abc thrd-xyz --with-replies --as alice   # Move message and all replies
fray mv msg-abc thrd-xyz --no-replies --as alice     # Move only this message (default)
```

Note: When moving to room, `new_home` is the literal string "room". Default is `--no-replies` (move only the specified message).

### Quote (fray-w5k3)

```bash
# Quote with commentary
fray post --as alice --quote msg-abc "This is the key insight from the discussion"

# Display shows quote inline
# > [original message preview]
# This is the key insight from the discussion
```

### Thread Listing Filters (fray-7rsr)

```bash
fray threads                    # Subscribed, not muted
fray threads --subscribed       # Explicit subscribed
fray threads --pinned           # Pinned threads
fray threads --activity         # Sorted by recent activity
fray threads --tree             # Tree view with nesting
fray threads --muted            # Only muted
fray threads --mine             # Threads I created
fray threads --all              # Everything
```

### Within-Thread Filters (fray-mgiz)

```bash
fray thread thrd-xyz --pinned      # Only pinned messages
fray thread thrd-xyz --by alice    # By agent
fray thread thrd-xyz --unread      # Uses read_to watermark (fray-vfv)
fray thread thrd-xyz --reactions   # Messages with reactions
fray thread thrd-xyz --with "keyword"  # Contains text
fray thread thrd-xyz --anchors     # Show anchors of sub-threads
```

### Cross-Thread Queries (fray-n28i)

These work with existing commands (no separate `fray query` needed):

```bash
fray faves --as alice              # All faved items
fray reactions --by alice          # Messages alice reacted to
fray reactions --to alice          # Reactions on alice's messages
```

### Thread Tree Display (fray-ao2f)

```bash
fray threads --tree
# design-review
# ├── ui-mockups          (3 pinned, 12 total)
# │   └── color-palette   (muted)
# ├── backend-api         ★ (subscribed)
# └── launch-checklist    📌 (pinned)
```

Indicators: ★ subscribed, 📌 pinned, (muted), message counts

### Agent Views (fray-oww1)

Existing `fray get alice` already provides agent-relevant updates. No separate inbox/watching/updates commands needed - keep it simple.

```bash
fray get alice              # Room + mentions + subscribed thread activity
fray @alice                 # Just mentions
```

### Saved Views (fray-cfxv) - DEFERRED TO v2

```bash
# Save current query
fray threads --pinned --activity --save "hot-threads"

# Use saved view
fray view hot-threads

# List saved views
fray views --as alice

# Delete saved view
fray view delete hot-threads --as alice
```

## Implementation Phases

### Phase 1: Core Curation Primitives

**Scope**: fray-dua7 (anchors), fray-eeor (pins), fray-jlw8 (rehome), fray-qsef (add-to-thread verification)

**Tasks**:
1. Add `anchor_message_guid`, `anchor_hidden`, `last_activity_at` fields to threads
2. Implement `fray thread anchor` command
3. Update `fray thread` display to show anchor at top
4. Add `fray_message_pins` table and JSONL records
5. Implement `fray pin`, `fray unpin` commands
6. Add `--pinned` filter to `fray thread`
7. Add `message_move` JSONL record type
8. Implement `fray mv` command (changes `home` field, supports --with-replies/--no-replies)
9. Update message queries to respect `home` changes
10. Verify `fray thread add` works as documented: adds to fray_thread_messages (reference), does NOT change message's `home` field

**Exit Criteria**:
- [ ] Thread anchors display and update
- [ ] `--hide/--unhide` toggles anchor visibility in parent thread
- [ ] `fray pin/unpin` work within threads
- [ ] `fray thread X --pinned` shows only pinned
- [ ] `fray mv` moves messages (home field changes, --with-replies works)
- [ ] `fray thread add` references without moving
- [ ] All changes tracked in JSONL
- [ ] `go test ./...` passes

### Phase 2: Thread Navigation Primitives

**Scope**: fray-50na (pin threads + fave threads), fray-qsbe (mute)

**Tasks**:
1. Add `fray_thread_pins` table and JSONL records
2. Implement `fray thread pin/unpin` (public)
3. Extend `fray fave` to handle threads (personal sidebar + subscribes)
4. Add `--pinned` and `--faved` filters to `fray threads`
5. Add `fray pins` command (all pinned items)
6. Add `fray_thread_mutes` table with expiry support
7. Implement `fray thread mute/unmute`
8. Update `fray threads` to exclude muted by default
9. Add implicit subscription: @mention or reply = subscribed

**Exit Criteria**:
- [ ] `fray thread pin/unpin` works (public)
- [ ] `fray fave thrd-xyz` adds to sidebar and subscribes (personal)
- [ ] `fray threads --pinned` and `--faved` work
- [ ] Mutes with TTL checked on query (auto-expire)
- [ ] Implicit subscription works (@mention/reply)
- [ ] `go test ./...` passes

### Phase 3: Attention Tracking

**Scope**: fray-dlvh (faves), fray-qrc4 (reactions), fray-w5k3 (quotes)

**Tasks**:
1. Add `fray_faves` table and JSONL records
2. Implement `fray fave/unfave` commands (fave = implicit subscribe)
3. Implement `fray faves` listing
4. Refactor reactions: no dedup (each session counts), add timestamps
5. Add `fray_reactions` table with compound PK (message_guid, agent_id, emoji, reacted_at)
6. Update reaction display: "👍x3" when multiple, "👍 alice" when single
7. Add `quote` field to messages
8. Implement `--quote` flag on `fray post`
9. Update message display to show quoted content inline

**Exit Criteria**:
- [ ] Faves work for threads and messages
- [ ] Faving a thread subscribes you
- [ ] Same agent can react multiple times (across sessions)
- [ ] Reactions display correctly (count vs name)
- [ ] Quotes display inline with commentary
- [ ] `go test ./...` passes

### Phase 4: Navigation & Views

**Scope**: fray-7rsr (thread filters), fray-mgiz (within-thread filters), fray-ao2f (tree display)

**Tasks**:
1. Add `--subscribed`, `--pinned`, `--activity`, `--muted`, `--mine` to `fray threads`
2. Implement `--tree` display with indicators
3. Add `--pinned`, `--by`, `--reactions`, `--with`, `--anchors` to `fray thread`
4. Add activity timestamp tracking for threads
5. Implement tree rendering with nesting indicators

**Exit Criteria**:
- [ ] All thread listing filters work
- [ ] Within-thread filters work
- [ ] Tree view shows hierarchy with indicators
- [ ] Activity sorting works
- [ ] `go test ./...` passes

### Phase 5: Advanced Features

**Scope**: fray-p3vo (4-level nesting), fray-n28i (cross-thread queries), fray-oww1 (agent views)

**Tasks**:
1. Add depth validation (max 4 levels from room)
2. Add `fray reactions --by/--to` for cross-thread reaction queries
3. Extend `fray get` to include subscribed thread activity (fray-oww1 - no new commands needed)

**Exit Criteria**:
- [ ] 4-level nesting works with validation
- [ ] `fray reactions --by alice` works across threads
- [ ] `fray get alice` includes subscribed thread activity
- [ ] `go test ./...` passes

## Deferred to v2

- **fray-cfxv (saved views)**: Named query presets - good idea but not essential for MVP

Note: Read/unread tracking is available via the `read_to` watermark system (fray-vfv). The `--unread` flag in fray-mgiz uses this existing infrastructure.

## Dependencies

- **Blocks**: fray-n0pg (mlld protocols) - protocols depend on thread curation being stable
- **Related**: fray-cbcs (room as coordination surface) - informs design patterns

## Testing Requirements

Each phase should include:
- JSONL append/read roundtrip
- SQLite rebuild from JSONL
- CLI command parsing
- Filter combinations
- Edge cases (empty results, not found, duplicates)

## Success Criteria

Epic complete when:
- [ ] 15 child issues closed (fray-cfxv deferred to v2)
- [ ] CLI reference updated in CLAUDE.md
- [ ] CHANGELOG entry written
- [ ] All tests pass
