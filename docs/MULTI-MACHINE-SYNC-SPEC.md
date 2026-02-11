# Multi-Machine Sync Specification

## Overview

Enable fray to work across multiple machines with eventual consistency. Each machine writes to its own JSONL files in a shared directory; a local SQLite cache merges all machines into a unified view. Agent identity lives in AAP and conversation threads; runtime config is machine-local.

## Design Principles

1. **Zero-conflict sync**: Each machine only writes to its own files
2. **Eventual consistency**: All machines converge to the same state
3. **Offline-first**: Works without network; syncs when available
4. **Identity in conversation**: Agent personality/context lives in `meta/agents/*` threads
5. **Runtime is local**: Invoke config, sessions, presence stay on each machine
6. **Simple sync backends**: Works with git, Syncthing, iCloud, Dropbox
7. **Tamper-evident**: Per-event signatures enable integrity verification
8. **Agent-scoped state**: Faves, roles, and preferences follow the agent identity, not machine

## Architecture

### The Three Layers

```
┌─────────────────────────────────────────────────────────────┐
│  ~/.config/aap/agents/{name}/identity.json                  │
│  WHO: GUID, public key, cryptographic identity              │
│  Syncs via: dotfiles (global to user)                       │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  .fray/shared/machines/*/*.jsonl                            │
│  WHAT: messages, threads, agent context (meta/agents/*)     │
│        ghost cursors, faves, role assignments               │
│  Syncs via: git, Syncthing, iCloud, Dropbox                 │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  .fray/local/                                               │
│  HOW (here): invoke config, sessions, presence, watermarks  │
│  Syncs via: nothing (machine-local, gitignored)             │
└─────────────────────────────────────────────────────────────┘
```

### Storage Structure

```
.fray/
  fray-config.json              # Channel config (syncs)
  
  shared/                       # SYNCS via chosen backend
    machines/
      laptop/
        messages.jsonl          # messages, message_update, reactions
        threads.jsonl           # threads, subscriptions, pins, mutes
        questions.jsonl         # questions
        agent-state.jsonl       # ghost_cursor, agent_fave, role_*
      server/
        messages.jsonl
        threads.jsonl
        questions.jsonl
        agent-state.jsonl
      desktop/
        ...
  
  local/                        # NEVER SYNCS (gitignored)
    machine-id                  # This machine's identifier
    runtime.jsonl               # agent registration, invoke, sessions, presence
    fray.db                     # SQLite cache (rebuilt from shared + local)
    fray.db-wal
    fray.db-shm
    daemon.lock
    daemon.log
```

### Data Classification

| Data Type | File | Location | Syncs? |
|-----------|------|----------|--------|
| Messages | messages.jsonl | shared/machines/*/ | ✓ |
| Message updates, reactions | messages.jsonl | shared/machines/*/ | ✓ |
| Threads, subscriptions | threads.jsonl | shared/machines/*/ | ✓ |
| Thread pins, mutes | threads.jsonl | shared/machines/*/ | ✓ |
| Questions | questions.jsonl | shared/machines/*/ | ✓ |
| Ghost cursors | agent-state.jsonl | shared/machines/*/ | ✓ |
| Faves | agent-state.jsonl | shared/machines/*/ | ✓ |
| Role assignments | agent-state.jsonl | shared/machines/*/ | ✓ |
| Agent registration | runtime.jsonl | local/ | ✗ |
| Invoke config | runtime.jsonl | local/ | ✗ |
| Sessions, heartbeats | runtime.jsonl | local/ | ✗ |
| Presence | runtime.jsonl | local/ | ✗ |
| Mention watermarks | runtime.jsonl | local/ | ✗ |
| SQLite cache | fray.db | local/ | ✗ |

### Why This Split?

**Ghost cursors sync** because they're handoff context. Cursors have explicit targets:
- `target: "opus@laptop"` - only laptop's opus uses this cursor
- `target: "opus@all"` - all machines' opuses use this cursor

When opus `/land`s on laptop with `/land --for opus@all`, all machines see the cursor.

**Faves and roles sync** because they're agent-conversation-state. If opus faves a thread, that preference should follow opus everywhere.

**Invoke config is local** because different machines may have different capabilities (GPU, model access, API keys).

**Sessions are local** because they're runtime state about what's happening on this machine right now.

**Presence is local** because it answers "is my local agent awake?" not "is this agent awake somewhere in the world?" For remote agents, the ack-in-chat pattern (agent posts when they see their @mention) provides sufficient coordination without requiring federated presence.

## Security & Integrity

### Event Signatures (Future)

Per-event signatures are deferred to a future phase. When implemented:

```jsonl
{"type":"message","id":"msg-abc",...,"sig":"base64-hmac-sha256"}
```

**Planned signature scheme:**
- Per-channel HMAC key stored in `fray-config.json` (generated at init, syncs with channel)
- Signature covers: `type + id + timestamp + body` (canonical JSON, sorted keys)
- Rebuild validates signatures; invalid events are logged and skipped
- Later: Per-event AAP signatures using agent's private key for non-repudiation

**Rationale for deferral:** Low threat model for trusted sync environments (git, Syncthing, iCloud). Interim integrity via per-file checksums is sufficient.

### Interim Integrity (Phase 2)

For trusted sync environments (git, Syncthing, iCloud), full per-event HMAC is deferred. Interim approach:

```json
// shared/checksums.json
{
  "laptop": {
    "messages.jsonl": {"sha256": "abc123...", "lines": 1234, "updated_at": 1234567890},
    "threads.jsonl": {"sha256": "def456...", "lines": 56, "updated_at": 1234567890}
  },
  "server": {
    "messages.jsonl": {"sha256": "789xyz...", "lines": 890, "updated_at": 1234567891}
  }
}
```

**Write ordering with concurrency protection:**
```go
func appendWithChecksum(dataPath, checksumPath string, data []byte) error {
    // 1. Append to data file (with its own flock)
    if err := atomicAppend(dataPath, data); err != nil {
        return err
    }

    // 2. Update checksum (separate flock on checksums.json)
    return updateChecksum(checksumPath, dataPath)
}

func updateChecksum(checksumPath, dataPath string) error {
    f, _ := os.OpenFile(checksumPath, os.O_RDWR|os.O_CREATE, 0644)
    defer f.Close()

    syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
    defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

    // Read, update entry for dataPath, write back
    checksums := readChecksums(f)
    checksums[dataPath] = computeChecksum(dataPath)
    return writeChecksums(f, checksums)
}
```

**Mismatch handling during rebuild:**
- Compare data file mtime vs checksum entry mtime
- If data file is newer than checksum: recalculate silently (write race or incomplete sync)
- If checksum is newer or same but hash differs: warn about potential corruption, recalculate

Using file mtime comparison avoids false alarms from clock skew between machines.

### File Integrity

**Atomic appends:**
```go
func atomicAppend(path string, data []byte) error {
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return err
    }
    defer f.Close()

    // Exclusive lock for the append
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
        return err
    }
    defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

    // Write data + newline
    if _, err := f.Write(append(data, '\n')); err != nil {
        return err
    }

    // Ensure durability
    return f.Sync()
}
```

**Recovery from corruption:**
- Each line must be valid JSON ending in newline
- Truncated final line (no newline) is discarded with warning
- Future: Optional checksum trailer per file for fast validation

### Machine ID Stability

Machine IDs must be stable and unique:
- Generated once at `fray init`, stored in `local/machine-id`
- Format: lowercase alphanumeric + hyphens, max 20 chars (e.g., `laptop`, `server-prod`)
- Collision detection: `fray init` on existing channel checks for ID conflicts
- Rename flow: `fray machine rename <old> <new>` with alias preservation

**Machine ID file format:**
```json
// local/machine-id
{
  "id": "laptop",
  "seq": 12345,
  "created_at": 1234567890
}
```

**Sequence counter atomicity:**
- Increment: flock file → read → increment → write → fsync → unlock
- Crash recovery: If seq missing/corrupt, scan all local JSONL files for max seq + 1
- Concurrent processes: flock guarantees single writer

## Agent Addressing

### Address Format (AAP-compatible)

| Format | Example | Meaning |
|--------|---------|---------|
| `@agent` | `@opus` | Local agent (must be registered locally to invoke) |
| `@agent.variant` | `@opus.frontend` | Agent with role specialization |
| `@agent@machine` | `@opus@laptop` | Specific machine's agent |
| `@agent@channel` | `@opus@fray` | Cross-channel reference |
| Full | `@opus.frontend@fray@laptop` | Full disambiguation (rare) |

### Mention Encoding

Mentions are encoded with machine scope at write time:

| You type | Stored as | Who processes |
|----------|-----------|---------------|
| `@opus` | `@opus@laptop` | Only laptop's opus |
| `@opus@server` | `@opus@server` | Only server's opus |
| `@opus@all` | `@opus@all` | Every machine's opus |

When you mention `@opus` without a machine qualifier, fray encodes it as `@opus@{this-machine}`. This means:
- Your local opus sees and responds to it
- Other machines' opuses ignore it (not addressed to them)
- No mention collision, no duplicate responses

To broadcast to all instances: explicitly use `@opus@all`.

### Daemon Processing

Each machine's daemon only processes mentions matching:
- `@{agent}@{this-machine}` - addressed to local agent
- `@{agent}@all` - broadcast to all machines

```go
func shouldProcessMention(mention string, localMachine string) bool {
    addr := parseMention(mention)
    return addr.Machine == localMachine || addr.Machine == "all"
}
```

### Display Rules

```go
// Show @agent@machine only when disambiguation needed
// Based on historical message origins, not live presence
func formatAgentDisplay(msg Message, ctx ConversationContext) string {
    if ctx.AgentHasMultipleOrigins(msg.FromAgent) {
        return fmt.Sprintf("@%s@%s", msg.FromAgent, msg.Origin)
    }
    return "@" + msg.FromAgent
}

func (ctx *ConversationContext) AgentHasMultipleOrigins(agentID string) bool {
    // Check if agent has messages from multiple origins in this channel
    // Uses synced message data, not live presence
    origins := ctx.db.GetDistinctOriginsForAgent(agentID)
    return len(origins) > 1
}
```

Note: Display can show `@opus` even though storage has `@opus@laptop` - the machine qualifier is implicit for local context. This uses historical message origins (synced data) rather than live presence (local-only), so display is deterministic across machines.

## Initialization Flow

### New Project (no existing .fray/)

```bash
$ fray init

Creating new fray channel...
  Channel name: cool-project
  Machine ID: laptop

✓ Created .fray/shared/machines/laptop/
✓ Created .fray/local/
✓ Initialized fray.db

Create your first agent? [Y/n]
  Agent name: opus
  Driver: [claude]

✓ Registered opus locally
✓ Created AAP identity

You can now:
  fray chat              # Open the chat UI
  fray agent create designer      # Create another agent
```

### Existing Project (synced .fray/ exists)

```bash
$ cd ~/projects/cool-thing
$ fray init

Found existing fray channel (synced from other machines)
  Channel: cool-thing (ch-abc123)
  Machines: laptop, server
  Storage version: 2

Checking machine ID...
  ✓ "desktop" is unique

Which agents do you want to run on "desktop"?

  [x] opus      (last active: laptop, 2h ago)  [code, review]
  [x] designer  (last active: laptop, 1d ago)  [design]
  [ ] reviewer  (last active: server, 3d ago)  [review]
  [ ] pm        (last active: laptop, 5d ago)  [planning]

Configure selected agents? [Y/n]

Setting up opus...
  Driver: [claude]
  Model: [default]

Setting up designer...
  Driver: [claude]
  Model: [default]

✓ Registered 2 agents on desktop
✓ Created .fray/local/
✓ Built cache from 2 synced machines

You can now:
  fray chat                    # Join the conversation
  fray agent add reviewer      # Add more agents later

Agents on other machines appear as @agent@machine
```

### Machine ID Collision

```bash
$ fray init

Found existing fray channel...
  Channel: cool-thing (ch-abc123)
  Machines: laptop, server

Checking machine ID...
  ✗ "laptop" already exists!

Choose a different machine ID:
  Machine ID: laptop2

  ✓ "laptop2" is unique
```

## JSONL Format Changes

### Messages (shared/machines/*/messages.jsonl)

Add `origin` and `seq` fields:

```jsonl
{"type":"message","id":"msg-abc","from_agent":"opus","origin":"laptop","body":"hello","seq":1,"ts":1234567890}
{"type":"message","id":"msg-def","from_agent":"opus","origin":"server","body":"world","seq":2,"ts":1234567891}
{"type":"message_update","id":"msg-abc","body":"hello edited","edit_reason":"typo","seq":3,"ts":1234567892}
{"type":"message_delete","id":"msg-abc","deleted_by":"opus","seq":4,"ts":1234567893}
```

**Deletions are sticky**: If any machine has a `message_delete` event for a GUID, that message is dead everywhere. This prevents resurrection after offline merges.

**GUID collisions**: Base36-8 provides ~2.8 trillion values; collisions are astronomically unlikely but possible with offline machines.

**Detection**: During rebuild, track seen GUIDs. If same GUID appears from different machines, log a warning with details.

**Handling**: Collisions are NOT auto-remediated (too complex, risk of breaking references). Instead:
1. Rebuild logs warning: `GUID collision: msg-abc from laptop and server`
2. Both messages are kept in database (last-write-wins for display)
3. User can manually remediate via `fray collision list` / `fray collision fix`

**Rationale**: Auto-reassigning GUIDs would require updating all references (replies, pins, faves, etc.) across all machines - a complex distributed operation. Manual remediation is safer for this edge case.

### Threads (shared/machines/*/threads.jsonl)

Thread events with tombstones:

```jsonl
{"type":"thread","id":"thrd-abc","name":"design-discussion","seq":1,"ts":1234567890}
{"type":"thread_subscribe","thread_id":"thrd-abc","agent_id":"opus","seq":2,"ts":1234567890}
{"type":"thread_unsubscribe","thread_id":"thrd-abc","agent_id":"opus","seq":3,"ts":1234567891}
{"type":"thread_mute","thread_id":"thrd-abc","agent_id":"opus","seq":4,"ts":1234567892}
{"type":"thread_unmute","thread_id":"thrd-abc","agent_id":"opus","seq":5,"ts":1234567893}
{"type":"thread_pin","thread_id":"thrd-abc","message_id":"msg-xyz","seq":6,"ts":1234567894}
{"type":"thread_unpin","thread_id":"thrd-abc","message_id":"msg-xyz","seq":7,"ts":1234567895}
{"type":"thread_delete","thread_id":"thrd-abc","seq":8,"ts":1234567896}
```

**Thread deletions are sticky**: Once deleted anywhere, stays deleted everywhere.

### Agent State (shared/machines/*/agent-state.jsonl)

Events that sync across machines:

```jsonl
{"type":"agent_descriptor","agent_id":"opus","display_name":"Opus","capabilities":["code","review"],"seq":1,"ts":1234567890}
{"type":"ghost_cursor","agent_id":"opus","target":"opus@laptop","home":"room","message_guid":"msg-xyz","seq":2,"ts":1234567890}
{"type":"ghost_cursor","agent_id":"opus","target":"opus@all","home":"room","message_guid":"msg-abc","seq":3,"ts":1234567891}
{"type":"agent_fave","agent_id":"opus","item_type":"thread","item_guid":"thrd-abc","seq":4,"ts":1234567890}
{"type":"fave_remove","agent_id":"opus","item_guid":"thrd-abc","seq":5,"ts":1234567891}
{"type":"role_hold","agent_id":"opus","role_name":"architect","seq":6,"ts":1234567890}
{"type":"role_release","agent_id":"opus","role_name":"architect","seq":7,"ts":1234567891}
{"type":"cursor_clear","agent_id":"opus","home":"room","seq":8,"ts":1234567892}
```

**Tombstones for all state:** Deletions are sticky across machines:
- `fave_remove` - unfave a thread/message
- `role_release` - release a held role
- `cursor_clear` - clear ghost cursor for a home

**Agent descriptors** are emitted when an agent first posts from a machine, enabling:
- Remote machine selection UI during `fray init`
- Display name resolution without AAP access
- Capability-based filtering

**Descriptor creation:**
- Automatically emitted on first message from an agent on a machine
- Migration scans messages.jsonl for unique `from_agent` values, emits descriptors
- Schema: `agent_id` (required), `display_name` (optional), `capabilities` (optional array)

**Sequence numbers** (`seq`) are per-machine counters that:
- Persist across restarts (stored in `local/machine-id` file)
- Provide tie-breaking when timestamps match
- Enable detection of missing events

**Faves and roles are agent-scoped**, not machine-scoped. If opus faves a thread on laptop, that fave applies to opus everywhere.

### Runtime (local/runtime.jsonl)

Machine-local agent state:

```jsonl
{"type":"agent","agent_id":"opus","invoke":{"driver":"claude","model":"opus-4"},"registered_at":1234567890}
{"type":"session_start","agent_id":"opus","session_id":"sess-xyz","started_at":1234567890}
{"type":"session_end","agent_id":"opus","session_id":"sess-xyz","exit_code":0,"ended_at":1234567900}
{"type":"agent_update","agent_id":"opus","presence":"active","mention_watermark":"msg-abc"}
```

## Rebuild Logic

### SQLite Cache Construction

```go
func RebuildDatabase(db DBTX, projectPath string) error {
    // 0. Check storage version
    config := readConfig(projectPath)
    if config.StorageVersion < 2 {
        return rebuildLegacy(db, projectPath) // Single-machine fallback
    }

    // 1. Merge shared conversation data from all machines
    messages, err := mergeFromAllMachines(projectPath, "messages.jsonl")
    if err != nil {
        return err
    }
    threads, _ := mergeFromAllMachines(projectPath, "threads.jsonl")
    questions, _ := mergeFromAllMachines(projectPath, "questions.jsonl")
    agentState, _ := mergeFromAllMachines(projectPath, "agent-state.jsonl")

    // 2. Build deletion set (sticky tombstones)
    deletedIDs := make(map[string]bool)
    for _, e := range messages {
        if e.Type == "message_delete" {
            deletedIDs[e.ID] = true
        }
    }

    // 3. Read local runtime (no merge needed)
    runtime := readLocal(projectPath, "runtime.jsonl")

    // 4. Discover all agents from conversation + descriptors
    allAgents := discoverAgentsFromMessages(messages)
    for _, e := range agentState {
        if e.Type == "agent_descriptor" {
            allAgents[e.AgentID] = AgentInfo{
                DisplayName:  e.DisplayName,
                Capabilities: e.Capabilities,
            }
        }
    }

    // 5. Build agents table
    for agentID, info := range allAgents {
        agent := Agent{ID: agentID, DisplayName: info.DisplayName}

        // Overlay local invoke config if registered here
        if local := runtime.GetAgent(agentID); local != nil {
            agent.Invoke = local.Invoke
            agent.Presence = local.Presence
            agent.MentionWatermark = local.MentionWatermark
        }

        insertAgent(db, agent)
    }

    // 6. Apply shared state (cursors, faves, roles)
    applyAgentState(db, agentState)

    // 7. Apply local sessions
    applySessions(db, runtime)

    // 8. Build messages table (skip deleted)
    for _, e := range messages {
        if e.Type == "message" && !deletedIDs[e.ID] {
            insertMessage(db, e)
        }
    }

    // 9. Build threads, questions tables
    // ... (existing logic, now with merged data)

    return nil
}

func mergeFromAllMachines(projectPath, filename string) ([]Event, error) {
    machines := discoverMachines(projectPath)
    var allEvents []TimestampedEvent
    seenGUIDs := make(map[string]TimestampedEvent)

    for _, machine := range machines {
        events, err := readJSONL(machine.Path, filename)
        if err != nil {
            return nil, fmt.Errorf("read %s/%s: %w", machine.ID, filename, err)
        }

        for _, e := range events {
            // Validate signature only if present (signatures are optional/future)
            if e.Sig != "" && !validateSignature(e, machine.HMACKey) {
                log.Warnf("invalid signature: %s/%s seq=%d", machine.ID, filename, e.Seq)
                continue
            }

            te := TimestampedEvent{
                Timestamp: e.Timestamp(),
                MachineID: machine.ID,
                Sequence:  e.Seq,
                Event:     e,
            }

            // Check for GUID collision (detect only, no auto-fix)
            if e.ID != "" {
                if existing, ok := seenGUIDs[e.ID]; ok && existing.MachineID != machine.ID {
                    log.Warnf("GUID collision: %s from %s and %s (keeping both, last-write-wins)",
                        e.ID, existing.MachineID, machine.ID)
                }
                seenGUIDs[e.ID] = te
            }

            allEvents = append(allEvents, te)
        }
    }

    // Sort by timestamp, then machine ID, then sequence (deterministic)
    sort.SliceStable(allEvents, func(i, j int) bool {
        if allEvents[i].Timestamp != allEvents[j].Timestamp {
            return allEvents[i].Timestamp < allEvents[j].Timestamp
        }
        if allEvents[i].MachineID != allEvents[j].MachineID {
            return allEvents[i].MachineID < allEvents[j].MachineID
        }
        return allEvents[i].Sequence < allEvents[j].Sequence
    })

    return unwrap(allEvents), nil
}
```

## Sync Configurations

### Git (developers)

```bash
# .fray/.gitignore
local/

# Workflow
git pull
fray rebuild
# ... work ...
git add .fray/shared/
git commit -m "fray sync"
git push
```

### Syncthing (continuous sync)

```bash
# Add .fray/shared/ to Syncthing folder
# Daemon watches for changes:
fray daemon --watch
```

### iCloud / Dropbox (personal machines)

```bash
# Option 1: Symlink shared directory
ln -s ~/Library/Mobile\ Documents/.../fray-sync/shared .fray/shared

# Option 2: Configure in fray
fray sync setup --icloud
fray sync setup --dropbox
fray sync setup --path ~/Dropbox/fray-sync
```

## CLI Commands

### Machine Management

```bash
fray machines                    # List all machines
fray machines --verbose          # Show file counts, last activity
```

Output:
```
MACHINE     STATUS    LAST WRITE    AGENTS
laptop      local     2 min ago     opus, designer
server      synced    15 min ago    opus, reviewer
desktop     synced    3 hours ago   designer
```

### Agent Management

```bash
fray agents                      # All agents (local + remote)
fray agents --local              # Only locally-invokable agents
fray agent add opus              # Register existing agent locally
fray agent add opus --driver codex --model o3
fray agent remove opus           # Stop running locally (keeps history)
fray agent create researcher     # Create brand new agent
```

### Sync Management

```bash
fray sync status                 # Show sync configuration
fray sync setup --icloud         # Configure iCloud sync
fray sync setup --dropbox        # Configure Dropbox sync  
fray sync setup --path DIR       # Configure custom sync path
fray rebuild                     # Force rebuild from all sources
```

## Migration

### From Single-Machine to Multi-Machine

```bash
$ fray migrate --multi-machine

Migrating to multi-machine mode...
  Machine ID: laptop

Copying files:
  messages.jsonl → shared/machines/laptop/messages.jsonl
  threads.jsonl → shared/machines/laptop/threads.jsonl
  questions.jsonl → shared/machines/laptop/questions.jsonl

Splitting agents.jsonl:
  → shared/machines/laptop/agent-state.jsonl (cursors, faves, roles)
  → local/runtime.jsonl (registration, invoke, sessions)

Creating commit point:
  → shared/.v2

Renaming legacy files:
  messages.jsonl → messages.jsonl.v1-migrated
  ...

✓ Migration complete
✓ Rebuilt fray.db

Next steps:
  1. Commit .fray/shared/ to git, or
  2. Run `fray sync setup --icloud` for cloud sync
```

**Idempotency**: If `shared/.v2` exists or any `*.v1-migrated` files exist, migration exits with "already migrated".

**Rollback procedure:**
- Failure before sentinel: remove `shared/` and `local/` directories, retry safe
- Failure after sentinel: migration is committed, manual cleanup if needed

### Legacy Compatibility

- Projects without `shared/` directory work in legacy single-machine mode
- All Read* functions fall back to reading from `.fray/*.jsonl` directly
- Migration is opt-in via `fray migrate --multi-machine`

### Version Gate

Once migrated, a version marker prevents legacy writers:

```json
// fray-config.json
{
  "channel_id": "ch-abc123",
  "storage_version": 2,
  "migrated_at": 1234567890
}
```

- `storage_version: 1` = legacy single-machine
- `storage_version: 2` = multi-machine mode

**Blocking legacy writes:**

1. **Rename legacy files**: Migration renames legacy files so old binaries can't find them:
   - `messages.jsonl` → `messages.jsonl.v1-migrated`
   - `threads.jsonl` → `threads.jsonl.v1-migrated`
   - `questions.jsonl` → `questions.jsonl.v1-migrated`
   - `agents.jsonl` → `agents.jsonl.v1-migrated`

2. **Sentinel file**: Migration creates `shared/.v2` marker.

3. **Hard check in write paths**: All append functions check storage_version before writing:
```go
func AppendMessage(projectPath string, msg Message) error {
    if GetStorageVersion(projectPath) >= 2 {
        // Must use shared/machines/{local}/messages.jsonl
        return appendToSharedMachine(projectPath, "messages.jsonl", msg)
    }
    // Legacy path
    return appendToLegacy(projectPath, "messages.jsonl", msg)
}
```

4. **Startup preflight**: If both `.fray/messages.jsonl` and `shared/.v2` exist, refuse to start with error explaining the conflict.

5. **Upgrade prompt**: If legacy binary detects `shared/` directory or `storage_version > 1`, it refuses to write and displays upgrade instructions.

This prevents split-brain where old clients continue appending to `.fray/messages.jsonl` while new clients use `shared/machines/*/`.

## Edge Cases

### Clock Skew

Messages from machines with different clocks may appear out of order.

Mitigations:
1. Use NTP (most machines do automatically)
2. Sequence numbers within machine provide secondary ordering
3. UI can warn if significant skew detected (>1 min difference)

### Mention Collision (Solved)

Mentions are encoded with machine scope at write time, so there's no collision:

- `@opus` typed on laptop → stored as `@opus@laptop` → only laptop processes
- `@opus@all` → all machines process (intentional broadcast)

### Offline Divergence

Machines working offline may create overlapping GUIDs (astronomically unlikely with base36-8) or conflicting thread states.

Resolution:
1. **GUIDs**: Detect-only. Log warning with details, keep both messages (last-write-wins for display). No auto-remediation - manual fix via future `fray collision fix` command.
2. **State**: Last-write-wins based on timestamp + sequence for deterministic tie-breaking.
3. **Deletions**: Sticky tombstones - once deleted anywhere, stays deleted everywhere.
4. **Future**: Optimistic locking via `if_version` field for conflict detection.

### Machine Rename/Retire

When a machine is renamed or retired, an alias map preserves mention routing:

```json
// fray-config.json
{
  "machine_aliases": {
    "old-laptop": "new-laptop",
    "retired-server": null
  }
}
```

**Alias resolution is wired into:**
- **Mention encoding**: `@opus@old-laptop` → stored as `@opus@new-laptop` (renamed machines only)
- **Daemon processing**: Checks aliases when matching mentions to local machine
- **Display**: Shows canonical name, not alias

```go
func resolveMachineAlias(config Config, machineID string) string {
    if alias, ok := config.MachineAliases[machineID]; ok && alias != "" {
        return alias // Renamed machine
    }
    return machineID // Keep original (including retired)
}

func isRetiredMachine(config Config, machineID string) bool {
    alias, ok := config.MachineAliases[machineID]
    return ok && alias == ""
}
```

**Retired machines:**
- Keep original machine ID in storage (remains displayable/auditable)
- Daemon skips processing mentions to retired machines
- Display shows original ID with retired indicator

- Mentions to `@opus@old-laptop` route to `@opus@new-laptop`
- Mentions to retired machines are displayed but not processed
- Alias map syncs with channel config

### Concurrent Agent Instances

**Presence is local-only by design.** Presence answers "is my local agent awake?" - it's about knowing whether a locally-prompted agent is active, idle, or offline. This is runtime state for the local daemon.

For remote agents (@opus@server), the **ack-in-chat pattern** provides sufficient coordination:
- You @mention an agent on another machine
- When that machine syncs and the daemon processes the mention, the agent responds
- The response in chat confirms the agent received and processed your mention

This is simpler and more reliable than federated presence, which would require real-time sync infrastructure. If we ever need real-time presence across machines, we'd likely adopt a proper chat protocol rather than building our own.

**Display uses message origins, not presence.** The `@agent@machine` display format is determined by whether an agent has posted from multiple origins in the conversation history - synced data that all machines can see consistently.

## Implementation Phases

### Phase 1: Storage Restructure
- [ ] Create shared/local directory structure
- [ ] Update all read functions to use `mergeFromAllMachines()`
- [ ] Update all write functions to use `getLocalMachineDir()`
- [ ] Add `origin` and `seq` fields to messages
- [ ] Implement per-machine sequence counter (persisted)
- [ ] Add `storage_version` to fray-config.json
- [ ] Mention encoding: `@opus` → `@opus@{machine}` at write time
- [ ] Alias resolution in mention encoding (for future machine rename)
- [ ] Migration command for existing projects

### Phase 2: Integrity & Safety
- [ ] Atomic append with flock + fsync
- [ ] Truncated line recovery
- [ ] Per-file SHA256 checksums (interim integrity, mtime comparison, flock protected)
- [ ] GUID collision detection (warn, don't auto-fix)
- [ ] `fray collisions` command to list detected collisions
- [ ] Tombstone events for all types (message, thread, fave, role, cursor)
- [ ] Machine ID collision detection at init
- [ ] Legacy write blocking (rename files + sentinel + startup preflight)
- [ ] *Deferred*: Per-event HMAC/AAP signatures

### Phase 3: Agent Onboarding
- [ ] `fray init` detects existing shared/
- [ ] Interactive agent selection UI with capabilities display
- [ ] `fray agent add/remove` commands
- [ ] Split agents.jsonl → runtime.jsonl + agent-state.jsonl
- [ ] `agent_descriptor` events for remote agent discovery

### Phase 4: Sync Backends
- [ ] Symlink setup for cloud storage
- [ ] fsnotify for auto-rebuild on file changes
- [ ] `fray sync` commands
- [ ] Git hook integration

### Phase 5: UI Polish
- [ ] `@agent@machine` display logic (based on message origins)
- [ ] Machine status in chat UI
- [ ] `fray machines` command
- [ ] `fray machine rename` (uses alias resolution from Phase 1)
- [ ] Sync status indicator

## Testing Strategy

### Unit Tests
- Merge ordering (timestamp, machine ID, sequence)
- Tombstone handling (message, thread, fave, role, cursor)
- Sequence counter atomicity (flock, increment, crash recovery)
- GUID collision detection (warn only, keep both)
- Alias/rename flows in mention resolution
- Retired machine handling (keep ID, skip processing)
- Signature validation skipped when sig absent

### Integration Tests
- Two temp directories simulating two machines
- Partial-line recovery (truncated JSONL)
- Machine ID collision at init
- Legacy mode regression (v1 still works)
- Migration v1 → v2 preserves all data
- Migration idempotency (second run is no-op)
- Migration rollback on failure

### Failure Cases
- Concurrent append from multiple processes (flock contention)
- Corrupt checksum detection (mtime comparison)
- Missing sequence after crash (scan + recover)
- Legacy binary blocked from writing to v2 project
- Partial migration recovery

## Future Enhancements

1. **Claim mechanism**: Prevent duplicate responses to mentions
2. **Real-time sync**: WebSocket/NATS for instant cross-machine updates
3. **Machine capabilities**: Tag machines with roles (GPU, production access)
4. **Selective sync**: Only sync certain threads to certain machines
5. **Conflict UI**: Show divergent edits, let user choose resolution
