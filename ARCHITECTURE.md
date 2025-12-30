# MM Architecture: GUID-Based Multi-Channel System

## Overview

MM uses a GUID-based architecture inspired by beads, enabling:
- Multi-machine coordination (git-mergeable)
- Cross-channel operations via a global channel registry
- Stable identifiers (GUIDs) with short display prefixes in UI
- Append-only JSONL storage (source of truth)
- SQLite cache (rebuildable from JSONL)

## Storage Structure

```
.mm/
  mm-config.json      # Project config (channel_id, known_agents, nicks)
  messages.jsonl      # Append-only source of truth
  agents.jsonl        # Append-only source of truth
  questions.jsonl     # Append-only source of truth
  threads.jsonl       # Append-only source of truth (threads + events)
  history.jsonl       # Pruned messages archive (optional)
  .gitignore          # Ignores *.db files
  mm.db               # SQLite cache (gitignored, rebuildable)
  mm.db-wal           # SQLite write-ahead log (gitignored)
  mm.db-shm           # SQLite shared memory (gitignored)

~/.config/mm/
  mm-config.json      # Global channel registry
```

## ID System

### Internal GUIDs (Stable, Never Change)

**Format:** `<type>-<8char-base36>` (0-9a-z)

- Messages: `msg-a1b2c3d4`
- Agents: `usr-x9y8z7w6`
- Channels: `ch-mmdev12`

**Why short GUIDs?**
- 8 chars base36 = large space (36^8)
- Readable in logs/debugging
- Not catastrophic if collision (just reassign)

### Display Prefixes (UI Only)

**Format:** `#<guid-prefix>`

- UI shows short prefixes like `#a1b2` / `#a1b2c` / `#a1b2c3`
- Length grows with message count (4/5/6 chars)
- No separate display-id table; the canonical ID is always the GUID

## JSONL Format

### messages.jsonl

Messages are append-only. Edits and deletes append `message_update` records.

```jsonl
{"type":"message","id":"msg-a1b2c3d4","channel_id":"ch-mmdev12","home":"room","from_agent":"adam","body":"@bob status","mentions":["bob"],"reactions":{"👍":["bob"]},"message_type":"agent","reply_to":null,"ts":1734612000,"edited_at":null,"archived_at":null}
{"type":"message_update","id":"msg-a1b2c3d4","body":"@bob updated status","edited_at":1734612600}
```

`home` controls visibility: `"room"` is surfaced, thread GUIDs are hidden in the room. `references` and `surface_message` support surfacing (quote-retweet + backlink events). `message_type` can be `surface` for surfaced posts.

### agents.jsonl

```jsonl
{"type":"agent","id":"usr-x9y8z7w6","name":"adam","global_name":"mm-adam","home_channel":"ch-mmdev12","created_at":"2025-12-19T09:00:00Z","active_status":"active","agent_id":"adam","status":"working","purpose":"developer relations","registered_at":1734608400,"last_seen":1734609000,"left_at":null}
```

`active_status` is legacy; current presence is derived from `last_seen`/`left_at` with a staleness window.

### questions.jsonl

```jsonl
{"type":"question","guid":"qstn-a1b2c3d4","re":"target market","from_agent":"party","to":"alice","status":"unasked","thread_guid":null,"asked_in":null,"answered_in":null,"created_at":1735500000}
{"type":"question_update","guid":"qstn-a1b2c3d4","status":"open","asked_in":"msg-x1y2z3w4"}
```

### threads.jsonl

```jsonl
{"type":"thread","guid":"thrd-b2c3d4e5","name":"market-analysis","parent_thread":null,"subscribed":["alice","bob"],"status":"open","created_at":1735500000}
{"type":"thread_subscribe","thread_guid":"thrd-b2c3d4e5","agent_id":"charlie","subscribed_at":1735500100}
{"type":"thread_message","thread_guid":"thrd-b2c3d4e5","message_guid":"msg-aaa","added_by":"alice","added_at":1735500200}
```

## Config Files

### Project config (.mm/mm-config.json)

```json
{
  "version": 1,
  "channel_id": "ch-mmdev12",
  "channel_name": "mm",
  "created_at": "2025-12-19T10:00:00Z",
  "known_agents": {
    "usr-x9y8z7w6": {
      "name": "adam",
      "global_name": "mm-adam",
      "home_channel": "ch-mmdev12",
      "created_at": "2025-12-19T09:00:00Z",
      "status": "working",
      "nicks": ["devrel"]
    }
  }
}
```

### Global config (~/.config/mm/mm-config.json)

```json
{
  "version": 1,
  "channels": {
    "ch-mmdev12": {
      "name": "mm",
      "path": "/Users/adam/dev/mini-msg"
    },
    "ch-party": {
      "name": "party",
      "path": "/Users/adam/dev/party"
    }
  }
}
```

### SQLite config (mm_config table)

The SQLite cache stores runtime config like `username`, `stale_hours`, and the
current channel metadata (`channel_id`, `channel_name`). This is distinct from
`.mm/mm-config.json` and the global registry.

## Agent Identity & Mentions

- Agent IDs are lowercase and may include numbers, hyphens, and dot segments
  (e.g., `alice`, `frontend-dev`, `alice.frontend`, `pm.3.sub`).
- Known agents are stored in `.mm/mm-config.json` to resolve nicks.
- Global names are stored as `channelName-agentID` for disambiguation.
- @mention prefix matching uses `.` as a separator; `@all` is a broadcast.
- `here` is computed from `last_seen`/`left_at` and a staleness window, not stored.

## Threads and Questions

- Threads are playlists. Messages have a single `home` and can be curated into additional threads via `mm_thread_messages`.
- Thread subscriptions live in `mm_thread_subscriptions` and are rebuilt from `threads.jsonl` (initial `subscribed` list + events).
- Questions live in `mm_questions`, optionally scoped to a thread via `thread_guid`.

## Channel System

### Registration Flow

```bash
cd ~/dev/mini-msg
mm init
# Prompts for a channel name, creates .mm/, registers in global config.
```

### Cross-Channel Operations

```bash
mm post --as adam "update" --in party
mm get --in party --last 10
mm chat party
```

**Channel context resolution:**
1. `--in <channel>` (matches by ID or name in global config)
2. Local `.mm/` project config

### Channel Commands

```bash
mm ls           # List registered channels
mm init         # Create .mm/, register globally
```

## Prune Strategy

### Cold Storage Pattern

```bash
mm prune              # Keep last 100 messages, archive rest
mm prune --all        # Wipe history.jsonl too
mm prune --keep 50    # Keep last 50 messages
```

**What happens:**
1. Read messages.jsonl
2. Append existing messages to history.jsonl (unless --all)
3. Keep last N in messages.jsonl
4. Rebuild SQLite from messages.jsonl

**Guardrails:**
- Requires a clean `.mm/` git state
- If the repo has an upstream, it must be in sync

## Chat UX

### Reply-to Syntax

**In chat:**
```
#abcd yes that works!
```

**Behind the scenes:**
- Parse `#abcd` → `msg-a1b2c3d4`
- Strip from body: "yes that works!"
- Set `reply_to` field: `msg-a1b2c3d4`

**Display:**
- Byline with a colored background (`@agent:`)
- Body tinted to match the byline color
- Reply context line (`↪ Reply to @agent: ...`) when available
- Meta line `#abcd` (dim) for the message GUID prefix
- Click a message to prefill a threaded reply (`#id ...`)
- Double-click a message to copy it

## Migration from v0.1.0

### Migration Command

```bash
mm migrate
```

**What it does:**
1. Copies `.mm/` to `.mm.bak/`
2. Generates GUIDs for agents/messages if missing
3. Creates `messages.jsonl` and `agents.jsonl`
4. Writes `.mm/mm-config.json`
5. Rebuilds SQLite from JSONL and restores read receipts
6. Registers the channel in global config

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
# mm rebuilds SQLite from JSONL as needed
mm get  # Sees new message
```

### Merge Conflicts

**JSONL is append-only:**
- messages.jsonl can be merged by appending both sides
- agents.jsonl conflicts are rare (GUID collision)

**SQLite rebuild:**
- After git merge, rebuild from JSONL
- GUIDs remain stable across machines

## Benefits

### For Users
- Cross-project coordination via registered channels
- Reference messages via stable GUIDs
- Multi-machine sync via git (no server needed)
- Prune old messages without losing history

### For Developers
- JSONL + SQLite cache (rebuildable)
- GUIDs avoid ID collisions across machines
- Display prefixes are derived, not stored
- Git-mergeable logs with clear provenance

## Release Flow

- Update `CHANGELOG.md` with the new version and changes.
- Keep `package.json` version in sync with the changelog.
- Merge to `main` triggers the release workflow.
- The workflow tags `vX.Y.Z`, builds artifacts via GoReleaser, publishes a GitHub release, updates the Homebrew formula, and publishes the npm package via trusted publishing.

### Required Secrets / Permissions

- `GITHUB_TOKEN` is provided automatically and is used for the GitHub release.
- `HOMEBREW_TAP_TOKEN` is optional; if unset, the workflow falls back to `GITHUB_TOKEN`.
  - If branch protection blocks GitHub Actions from pushing formula updates, add a PAT as `HOMEBREW_TAP_TOKEN` or allow the Actions bot to push.
- npm trusted publishing uses GitHub OIDC; no `NPM_TOKEN` is required.

### Forcing a Release

- Manually run the `Release` workflow and set `force=true` to publish even if the tag already exists.
