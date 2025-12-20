# MM Architecture: GUID-Based Multi-Channel System

## Overview

MM uses a GUID-based architecture inspired by beads, enabling:
- Multi-machine coordination (git-mergeable)
- Cross-channel messaging (reference party messages from mm project)
- Stable identifiers (GUIDs) with UX-friendly aliases (short IDs)
- Append-only JSONL storage (source of truth)
- SQLite cache (rebuildable from JSONL)

## Storage Structure

```
.mm/
  messages.jsonl      # Append-only source of truth (committed to git)
  agents.jsonl        # Agent registrations (committed to git)
  history.jsonl       # Pruned messages archive (optional)
  .gitignore          # Ignores *.db files
  mm.db               # SQLite cache (gitignored, rebuildable)
  mm.db-wal           # SQLite write-ahead log (gitignored)
  mm.db-shm           # SQLite shared memory (gitignored)

~/.config/mm/
  mm-config.json      # Global channel and agent registry
```

## ID System

### Internal GUIDs (Stable, Never Change)

**Format:** `<type>-<8char-base58>`

- Messages: `msg-a1b2c3d4`
- Agents: `usr-x9y8z7w6`
- Channels: `ch-mmdev12`

**Why short GUIDs?**
- 8 chars base58 = 2^47 combinations (~140 trillion)
- Collision risk: negligible for human-scale use
- Readable in logs/debugging
- Not catastrophic if collision (just reassign)

### Display IDs (UX Layer, Can Change)

**Format:** `<channel>-<sequence>`

- Messages: `#mm-42` → `msg-a1b2c3d4`
- Agents: `@devrel` → `usr-x9y8z7w6` (in home channel)
- Channels: `mm` → `ch-mmdev12`

**Mapping:**
```sql
CREATE TABLE mm_display_ids (
  guid TEXT PRIMARY KEY,
  channel_id TEXT,
  short_id INTEGER,        -- Sequential counter (resets after prune)
  display_id TEXT,         -- mm-42
  valid_from TIMESTAMP,
  valid_until TIMESTAMP    -- NULL = current
);
```

**After prune:**
- Short IDs reset: `#mm-1`, `#mm-2`, ...
- GUIDs unchanged: `msg-a1b2c3d4` still valid
- Old display IDs (`#mm-501`) archived with `valid_until` timestamp

## JSONL Format

### messages.jsonl

```jsonl
{"type":"message","id":"msg-a1b2c3d4","agent_id":"usr-x9y8z7w6","body":"@#msg-b2c3 works!","mentions":["usr-y8z7"],"reply_to":"msg-b2c3","created_at":"2025-12-19T10:00:00Z","channel_id":"ch-mmdev"}
```

### agents.jsonl

```jsonl
{"type":"agent","id":"usr-a1b2c3d4","name":"devrel","global_name":"mm-devrel","home_channel":"ch-mmdev","created_at":"2025-12-19T09:00:00Z","status":"active"}
{"type":"agent","id":"usr-x9y8","name":"adam","global_name":"adam","home_channel":null,"created_at":"2025-12-19T09:00:00Z","status":"active"}
```

### Tombstones (Not Used)

We skip tombstones. Messages are immutable. If you want to "retract" a message, post a new one.

Pruning physically removes old messages (moves to history.jsonl).

## Global Config

**Location:** `~/.config/mm/mm-config.json`

```json
{
  "version": 1,
  "channels": {
    "ch-mmdev": {
      "name": "mm",
      "path": "/Users/adam/dev/mini-msg",
      "created_at": "2025-12-19T10:00:00Z"
    },
    "ch-party": {
      "name": "party",
      "path": "/Users/adam/dev/party",
      "created_at": "2025-12-19T11:00:00Z"
    }
  },
  "agents": {
    "usr-adam123": {
      "name": "adam",
      "home_channel": null,
      "nicks": {}
    },
    "usr-devrel5": {
      "name": "mm-devrel",
      "home_channel": "ch-mmdev",
      "nicks": {
        "ch-mmdev": "devrel",
        "ch-party": "mm-devrel"
      }
    }
  },
  "current_channel": "ch-mmdev"
}
```

## Agent Identity

### Two Types

**Global Agents** (humans, cross-project bots)
```bash
mm new adam --global
# Appears as @adam everywhere
# No home channel
```

**Channel-Homed Agents** (project specialists)
```bash
mm new devrel
# Default behavior (home = current channel)
# Globally: usr-devrel5 → mm-devrel
# In mm channel: @devrel (auto-nick)
# In party channel: @mm-devrel (full global name)
```

### Nick Resolution

**Auto-nicks (channel-homed agents):**
- In home channel: `@devrel` → `usr-devrel5`
- In other channels: `@mm-devrel` → `usr-devrel5`

**Custom nicks:**
```bash
mm nick @mm-devrel --as helper --in party
# Now @helper → usr-devrel5 in party channel
```

## Channel System

### Registration Flow

```bash
# First time in a new project
cd ~/dev/mini-msg
mm init

# Prompts:
# Channel name? [mini-msg]: mm
# ✓ Registered channel ch-mmdev12 as 'mm'
# ✓ Created .mm/ directory
# ✓ Added to ~/.config/mm/mm-config.json
```

### Cross-Channel Operations

**From anywhere:**
```bash
# Explicit channel
mm post @party-dev "update" --in party
mm get --in party --last 10

# Global message references
mm thread @#party-42

# Channel switching
mm use party
mm post @dev "hi"  # Now uses party context
```

**Channel context resolution:**
1. `--in <channel>` flag (explicit)
2. Current channel from `mm use` (saved in config)
3. Channel from current directory (if .mm/ exists)
4. Error: "No channel context"

### Channel Commands

```bash
mm ls                          # List all channels, show current
mm init [channel-name]         # Create .mm/, register globally
mm use <channel>               # Switch current channel context
mm rename-channel <old> <new>  # Rename channel display name
```

## Prune Strategy

### Cold Storage Pattern

```bash
mm prune              # Keep last 100 messages, archive rest
mm prune --all        # Wipe history.jsonl too
mm prune --keep 50    # Keep last 50 messages
```

**What happens:**
1. Read messages.jsonl (500 messages)
2. Append all to history.jsonl
3. Keep last N in messages.jsonl
4. Reset short_id counter to 1
5. Rebuild SQLite from messages.jsonl

**Old references still work:**
- GUIDs unchanged: `msg-a1b2c3d4` still valid
- Display IDs archived: `#mm-501` marked with `valid_until`
- Can resolve old IDs via history.jsonl (optional feature)

## Chat UX

### Reply-to Syntax

**In chat:**
```
> @#42 yes that works!
```

**Behind the scenes:**
- Parse `@#42` → `msg-b2c3d4e5`
- Strip from body: "yes that works!"
- Set `reply_to` field: `msg-b2c3d4e5`

**Display:**
```
[2m ago] @devrel: "yes that works!" @#mm-43
  ↪ Reply to @adam: "what do you think about..."
```

**Format:**
- Reply context: dimmed, indented, truncated (~50 chars)
- Permalink `@#mm-43`: dimmed, end of line
- In JSON: full body includes `@#42`
- In chat display: stripped, shown as reply context

### First-time Flow

```
$ mm chat

Welcome to mm chat!

Channel name for this project? [mini-msg]: mm
✓ Registered channel: ch-mmdev12 as 'mm'

Your name? [adam]: adam
✓ Registered as global agent: usr-adam123 (@adam)

Joining #mm...

#mm

[just now] @adam: "hello!" @#mm-1
```

## Migration from v0.1.0

### Breaking Changes

- Message IDs: numeric → GUID-based
- Agent IDs: `alice.1` → `usr-xxxxx` (display: `@alice`)
- Channel required: must run `mm init` to set channel

### Migration Command

```bash
mm migrate

# What it does:
# 1. Generates GUIDs for existing messages/agents
# 2. Creates messages.jsonl, agents.jsonl from current SQLite
# 3. Registers channel in global config
# 4. Rebuilds SQLite with GUID schema
# 5. Backs up old .mm/ to .mm.bak/
```

### No Data to Preserve

Since this is pre-release (v0.1.0 → v0.2.0):
- Simple: Delete `.mm/` and re-init
- Or: Run `mm migrate` for testing migration flow

## Multi-Machine Sync

### Git Workflow

```bash
# Machine A
mm post @dev "update"
git add .mm/messages.jsonl .mm/agents.jsonl
git commit -m "Add message"
git push

# Machine B
git pull
# mm automatically detects JSONL changes, rebuilds SQLite

mm get  # Sees new message
```

### Merge Conflicts

**JSONL is append-only:**
- No conflicts in messages.jsonl (just append both)
- Conflicts in agents.jsonl rare (different GUIDs)
- Beads has proven this pattern works

**SQLite rebuild:**
- After git merge, rebuild from JSONL
- Short IDs recalculated (sequential)
- GUIDs stable across machines

## Implementation Phases

### Phase 0: GUID Foundation (Week 1)
- P0.1: Add GUID columns to schema ✓ Ready
- P0.2: JSONL storage layer (blocks: P0.1)
- P0.3: SQLite rebuild (blocks: P0.2)
- P0.4: Channel GUID system (blocks: P0.1)
- P0.5: Agent GUID registry (blocks: P0.4)
- P0.6: Display ID mapping (blocks: P0.5)
- P0.7: Migration command (blocks: P0.6)

### Phase 1: Core Features (Week 2)
- P1.1: JSON output (mm here, mm history, mm between)
- P1.2: Chat reply-to with @#id
- P1.3: Show message IDs in chat
- P1.4: Cross-channel operations (--in flag)
- P1.5: Prune with cold storage
- P1.6: mm ls command

### Phase 2: Polish (Week 3+)
- P2.1: Nick management (mm nick, mm nicks, mm whoami)
- P2.2: Shell autocomplete
- Docs: Update README, CHANGELOG for v0.2.0

## Benefits

### For Users
- Cross-project coordination without `cd ~/dir && mm post`
- Reference messages globally: `@#party-42`
- Multi-machine sync via git (no server needed)
- Prune old messages without losing references

### For Developers
- Beads-proven architecture (JSONL + SQLite)
- GUIDs solve ID collision across machines
- Display IDs give UX-friendly numbers
- Git-mergeable (append-only JSONL)

### For Party Project
- Lightweight routing summaries (mm get --last 10 --json)
- Agent conversation history (mm history alice --json)
- Cross-channel awareness (mm @party-dev --in party)
- Stable IDs for context assembly

## Next Steps

Start with P0.1: Add GUID columns to schema (bdm-7kr)
```bash
bd show bdm-7kr
bd update bdm-7kr --status=in_progress
```
