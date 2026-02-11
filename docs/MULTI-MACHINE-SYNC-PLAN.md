# Multi-Machine Sync Implementation Plan

## Overview

Enable fray to work across multiple machines with eventual consistency. Each machine writes to its own JSONL files in a shared directory; a local SQLite cache merges all machines into a unified view.

## Target Architecture

```
.fray/
  fray-config.json              # Syncs (adds storage_version: 2)

  shared/                       # SYNCS via git/Syncthing/iCloud
    machines/
      laptop/
        messages.jsonl          # messages, reactions, pins
        threads.jsonl           # threads, subscriptions, mutes
        questions.jsonl
        agent-state.jsonl       # ghost_cursor, faves, roles
      server/
        ...

  local/                        # NEVER SYNCS (gitignored)
    machine-id                  # JSON: {"id": "laptop", "seq": 12345, "created_at": ...}
    runtime.jsonl               # agent registration, invoke, sessions, presence
    fray.db                     # SQLite cache
```

## Implementation Phases

### Phase 1: Storage Restructure (Foundation)

**Objective**: Create directory structure, update read/write routing, implement migration

#### 1.1 Core Path Helpers
**File**: `internal/db/jsonl.go`

Add:
- `GetStorageVersion(projectPath) int` - read from fray-config.json
- `IsMultiMachineMode(projectPath) bool` - check storage_version >= 2
- `GetLocalMachineID(projectPath) string` - read from local/machine-id
- `GetSharedMachinesDirs(projectPath) []string` - list all machine directories
- `GetLocalMachineDir(projectPath) string` - this machine's shared dir
- `GetLocalRuntimePath(projectPath) string` - local/runtime.jsonl path
- `GetNextSequence(projectPath) int64` - per-machine seq counter

#### 1.2 Update Read Functions (Merge Logic)
**File**: `internal/db/jsonl_read.go`

For each Read* function, add multi-machine branch:
```go
func ReadMessages(projectPath string) ([]MessageJSONLRecord, error) {
    if IsMultiMachineMode(projectPath) {
        return readMessagesMerged(projectPath)  // merge from all machines
    }
    return readMessagesLegacy(projectPath)  // existing code
}
```

**Merge pattern for shared data**:
- Read from all `shared/machines/*/messages.jsonl`
- Track deletions (sticky tombstones)
- Sort by (timestamp, machineID, seq) for deterministic order

**Local-only data** (`ReadAgents`):
- Read from `local/runtime.jsonl` only

#### 1.3 Update Write Functions (Routing)
**File**: `internal/db/jsonl_append.go`

Route writes:

| Function | Target (multi-machine) |
|----------|------------------------|
| `AppendMessage`, `AppendMessageUpdate`, `AppendReaction` | `shared/machines/{local}/messages.jsonl` |
| `AppendThread*`, `AppendMessagePin/Unpin` | `shared/machines/{local}/threads.jsonl` |
| `AppendQuestion*` | `shared/machines/{local}/questions.jsonl` |
| `AppendGhostCursor`, `AppendAgentFave`, `AppendRole*` | `shared/machines/{local}/agent-state.jsonl` |
| `AppendAgent`, `AppendSession*`, `AppendPresence*`, `AppendWake*` | `local/runtime.jsonl` |

Add `origin` and `seq` fields to message records.

#### 1.4 Mention Encoding + Alias Resolution
**File**: `internal/core/mentions.go`

Mention encoding at write time:
- `@opus` → `@opus@{local-machine}`
- Check alias map for renamed machines (retired machines keep original ID)

```go
func encodeMention(config Config, mention string, localMachine string) string {
    if !hasMachineQualifier(mention) {
        return mention + "@" + localMachine
    }
    // Resolve aliases for renamed machines only (not retired)
    parts := parseMention(mention)
    if alias, ok := config.MachineAliases[parts.Machine]; ok && alias != "" {
        parts.Machine = alias
    }
    return parts.String()
}
```

#### 1.5 Update Rebuild Logic
**File**: `internal/db/jsonl_rebuild.go`

Modify `RebuildDatabaseFromJSONL()`:
- Check storage_version, branch to legacy or multi-machine
- Use Read* functions (which now handle merge)
- Apply agent descriptors from remote machines
- Overlay local invoke config

#### 1.5 Update mtime Check
**File**: `internal/db/open.go`

Check all machine directories + local/runtime.jsonl for changes.

#### 1.6 Migration Command
**File**: `internal/command/migrate.go` (extend existing)

`fray migrate --multi-machine`:

**Idempotency guard**: If `shared/.v2` exists or any `*.v1-migrated` files exist, exit with "already migrated" message.

**Steps:**
1. Prompt for machine ID (or derive from hostname)
2. Create `shared/machines/{id}/` and `local/`
3. Copy messages.jsonl, threads.jsonl, questions.jsonl to shared/
4. Split agents.jsonl:
   - ghost_cursor, faves, roles → `shared/machines/{id}/agent-state.jsonl`
   - agent, sessions, presence → `local/runtime.jsonl`
5. Write machine-id file
6. **Create `shared/.v2` sentinel (COMMIT POINT)**
7. Rename legacy files (prevents old binaries from writing):
   - `messages.jsonl` → `messages.jsonl.v1-migrated`
   - `threads.jsonl` → `threads.jsonl.v1-migrated`
   - `questions.jsonl` → `questions.jsonl.v1-migrated`
   - `agents.jsonl` → `agents.jsonl.v1-migrated`
8. Update fray-config.json: `storage_version: 2`
9. Rebuild database

**Rollback procedure:**
- If failure before step 6 (no sentinel): remove `shared/` and `local/` directories
- If failure after step 6 (sentinel exists): migration is committed, manual cleanup needed
- On rerun: idempotency guard detects sentinel/migrated files, exits cleanly

### Phase 2: Integrity & Safety

#### 2.1 Atomic Append
**File**: `internal/db/jsonl_append.go`

```go
func atomicAppend(path string, data []byte) error {
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil { return err }
    defer f.Close()

    syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
    defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

    f.Write(append(data, '\n'))
    return f.Sync()
}
```

- Truncated line recovery: skip partial final line with warning

#### 2.2 Sequence Counter
**File**: `internal/db/jsonl.go`

```go
func GetNextSequence(projectPath string) (int64, error) {
    path := filepath.Join(projectPath, ".fray", "local", "machine-id")
    // flock → read JSON → increment seq → write → fsync → unlock
    // Crash recovery: if corrupt, scan local JSONL for max seq + 1
}
```

#### 2.3 Tombstones (All Types)
- `message_delete` - sticky message deletion
- `thread_delete` - sticky thread deletion
- `fave_remove` - unfave
- `role_release` - release held role
- `cursor_clear` - clear ghost cursor

#### 2.4 Interim Integrity
**File**: `shared/checksums.json`

Per-file SHA256 checksums, updated on each append.

**Write ordering with flock:**
1. Append event to data file (flock on data file)
2. Update checksums.json (separate flock on checksums.json)

Both operations use exclusive locks to handle concurrent writers.

**Mismatch handling:**
- Compare data file mtime vs checksum entry mtime
- Data newer than checksum: recalculate silently (write race)
- Checksum newer or same but hash differs: warn, recalculate

#### 2.5 GUID Collision Detection
- Detection: During rebuild, track seen GUIDs per machine
- On collision: Log warning with details, keep both (last-write-wins for display)
- Store collisions in `local/collisions.json` for CLI access
- NO auto-remediation (too complex, risk of breaking references)

#### 2.6 Collision CLI
**File**: `internal/command/collisions.go`

```bash
fray collisions              # List detected GUID collisions
fray collisions --clear      # Clear collision log after manual review
```

Output shows: GUID, machines involved, timestamps, affected content preview.

#### 2.7 Legacy Write Blocking
- Migration renames legacy files: `*.jsonl` → `*.jsonl.v1-migrated`
- Create `shared/.v2` sentinel on migration
- Startup preflight: If both `.fray/messages.jsonl` and `shared/.v2` exist, fail with error
- All append functions check `storage_version >= 2` before writing
- Refuse to write to legacy paths if v2, display upgrade prompt

#### 2.8 Machine ID Collision
- `fray init` on existing channel checks for ID conflicts
- Prompt for different ID if collision detected

*Deferred*: Per-event HMAC/AAP signatures (low threat model for trusted sync)

### Phase 3: Agent Onboarding

#### 3.1 Join Flow
- `fray init` detects existing `shared/` directory
- Check machine ID collision
- Show agent selection UI with capabilities from descriptors

#### 3.2 Agent Descriptor Events
**Location**: `shared/machines/{local}/agent-state.jsonl`

```jsonl
{"type":"agent_descriptor","agent_id":"opus","display_name":"Opus","capabilities":["code","review"],"seq":1,"ts":...}
```

**Creation triggers**:
- On first message from agent on this machine
- Migration scans messages.jsonl for unique `from_agent`, emits descriptors

**Schema**: `agent_id` (required), `display_name` (optional), `capabilities` (optional array)

#### 3.3 Commands
- `fray agent add <name>` - register remote agent locally
- `fray agent remove <name>` - stop running locally (keeps history)

### Phase 4: Sync Backends

- `fray sync setup --icloud|--dropbox|--path DIR`
- fsnotify watcher for auto-rebuild
- Git post-merge hook

### Phase 5: UI Polish

#### 5.1 Display Logic
- `@agent@machine` display when agent has multiple origins (based on message history, not presence)
- Uses `GetDistinctOriginsForAgent()` query

#### 5.2 Machine Commands
- `fray machines` - list all machines with status
- `fray machine rename <old> <new>` - rename with alias preservation

#### 5.3 Machine Rename Command
**Prerequisite**: Alias resolution already wired in Phase 1.4

`fray machine rename <old> <new>`:
1. Add entry to `machine_aliases` in fray-config.json
2. Rename `shared/machines/<old>/` to `shared/machines/<new>/`
3. Update `local/machine-id` if renaming local machine
4. Rebuild database

All existing mentions using old ID continue to work via alias resolution.

## Files to Modify

| File | Changes |
|------|---------|
| `internal/db/jsonl.go` | Storage config types, path helpers, seq counter |
| `internal/db/jsonl_read.go` | Multi-machine merge for all Read* functions |
| `internal/db/jsonl_append.go` | Route writes to shared/ or local/ |
| `internal/db/jsonl_rebuild.go` | Version check, multi-machine rebuild path |
| `internal/db/open.go` | Multi-directory mtime check |
| `internal/command/migrate.go` | `--multi-machine` subcommand |
| `internal/command/init.go` | Detect shared/, join existing project |
| `internal/core/project.go` | Multi-machine structure detection |

## Testing Strategy

### Unit Tests
- Merge ordering (timestamp, machine ID, sequence)
- Tombstone handling (message, thread, fave, role, cursor)
- Sequence counter atomicity (flock, increment, crash recovery)
- GUID collision detection (warn only, keep both)
- Alias/rename flows in mention resolution
- Retired machine handling (keep ID, skip processing)

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

## Verification

1. Run `fray migrate --multi-machine` on test project
2. Verify directory structure created correctly
3. Run `fray chat` and post messages
4. Manually copy shared/ to simulate sync
5. Run `fray rebuild` and verify merged view
6. Run existing tests: `go test ./...`

## Decisions Made

1. **Machine ID format**: Lowercase alphanumeric + hyphens, max 20 chars (e.g., `work-laptop`).
2. **Machine ID file**: JSON with id, seq counter, created_at. Seq incremented atomically via flock.
3. **Sequence crash recovery**: Scan local JSONL for max seq + 1 if counter corrupt/missing.
4. **HMAC signatures**: Deferred. Low threat model for trusted sync. Interim: per-file SHA256 checksums.
5. **Checksum atomicity**: Data append first, checksum update second. Use mtime comparison for race detection.
6. **Invalid signature handling**: (When implemented) Skip event with warning, don't fail rebuild.
7. **Sessions**: Stay in local/runtime.jsonl - never sync. Solves stale session_id by design.
8. **Presence**: Local-only. Remote agents use ack-in-chat pattern.
9. **Display logic**: Uses message origins (synced), not presence (local).
10. **Tombstones**: All types (message, thread, fave, role, cursor). Sticky - once deleted anywhere, deleted everywhere.
11. **Legacy write blocking**: Rename files + sentinel + startup preflight.
12. **Faves/roles**: Agent-scoped, not machine-scoped.
13. **GUID collisions**: Detect and warn only, no auto-remediation (too complex).
14. **Alias resolution timing**: Phase 1 with mention encoding, before machine rename ships.
15. **Retired machines**: Keep original ID in storage (auditable), daemon skips processing.
16. **Checksum mismatch**: Use file mtime comparison to avoid clock skew false alarms.
17. **Signature validation**: Skip if sig field absent (signatures are optional/future).
18. **Migration idempotency**: Guard on sentinel/migrated files, rollback on failure.
