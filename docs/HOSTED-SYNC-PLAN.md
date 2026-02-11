# Hosted Sync Plan (Git-Compatible)

This plan sketches the hosted sync service and its client sync daemon while preserving git sync as a supported option.

---

## 1) Minimal Query DB Schema

This read model supports the web UI and fast API queries. It is rebuilt from JSONL streams and is **not** the source of truth.

### messages
```
messages (
  guid TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  ts BIGINT NOT NULL,
  home TEXT NOT NULL DEFAULT 'room',
  from_agent TEXT NOT NULL,
  origin TEXT,                       -- machine id
  session_id TEXT,
  body TEXT NOT NULL,
  mentions JSONB NOT NULL DEFAULT '[]',
  fork_sessions JSONB,
  type TEXT NOT NULL DEFAULT 'agent',
  references TEXT,
  surface_message TEXT,
  reply_to TEXT,
  quote_message_guid TEXT,
  edited_at BIGINT,
  archived_at BIGINT,
  reactions JSONB NOT NULL DEFAULT '{}'
)

INDEX (channel_id, ts DESC)
INDEX (channel_id, home, ts DESC)
INDEX (channel_id, from_agent)
INDEX (reply_to)
INDEX (archived_at)
```

### threads
```
threads (
  guid TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  name TEXT NOT NULL,
  parent_thread TEXT,
  status TEXT NOT NULL DEFAULT 'open',
  type TEXT NOT NULL DEFAULT 'standard',
  created_at BIGINT NOT NULL,
  anchor_message_guid TEXT,
  anchor_hidden BOOLEAN NOT NULL DEFAULT false,
  last_activity_at BIGINT
)

INDEX (channel_id, parent_thread)
INDEX (channel_id, status)
INDEX (channel_id, last_activity_at DESC)
```

### questions
```
questions (
  guid TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  re TEXT NOT NULL,
  from_agent TEXT NOT NULL,
  to_agent TEXT,
  status TEXT NOT NULL DEFAULT 'unasked',
  thread_guid TEXT,
  asked_in TEXT,
  answered_in TEXT,
  options JSONB NOT NULL DEFAULT '[]',
  created_at BIGINT NOT NULL
)

INDEX (channel_id, status)
INDEX (channel_id, thread_guid)
INDEX (channel_id, to_agent)
```

Notes:
- `channel_id` keeps multi‑channel support explicit.
- JSONB can be plain JSON if using SQLite for the read model.
- Full‑text search can be added on `messages.body`.

---

## 2) Reference Sync Daemon (Hosted ↔ Local .fray/shared)

A local process that keeps `.fray/shared/` in sync with the hosted backend. It mirrors the append‑only JSONL streams.

### Responsibilities
- **Push** local new lines to the hosted service.
- **Pull** new lines from hosted and append locally.
- Maintain per‑stream cursors in `.fray/local/sync-state.json`.
- Avoid rewriting history; append‑only.

### Files
- **Local shared streams**: `.fray/shared/machines/<id>/*.jsonl`
- **Local state**: `.fray/local/sync-state.json`

### Sync State Format (example)
```json
{
  "channel_id": "ch-abc123",
  "streams": {
    "laptop/messages.jsonl": { "line": 1024, "sha256": "..." },
    "server/threads.jsonl":  { "line": 88,   "sha256": "..." }
  }
}
```

### Loop (High Level)
1. **Manifest fetch**: request `streams` metadata from the server.
2. **Push local deltas**:
   - detect new lines since last cursor
   - push in batches with base `(line_count, sha256, last_seq)`
3. **Pull remote deltas**:
   - request lines since cursor
   - append to local file with flock + fsync
4. Update sync state atomically.

### Local Change Detection
Options (both acceptable):
- **fsnotify** on `.fray/shared/machines/<local>` for low‑latency push.
- **polling** with mtime checks (e.g., every 2–5s).

### Push API (reference)
```
POST /v1/sync/push
{
  channel_id,
  machine_id,
  file,                       // messages.jsonl | threads.jsonl | ...
  base: { line_count, sha256, last_seq },
  lines: ["{...}", "{...}"],
  idempotency_key
}
```

### Pull API (reference)
```
POST /v1/sync/pull
{
  channel_id,
  cursors: [{ machine_id, file, line_offset }]
}
```

### Conflict Handling
- If server rejects a push due to base mismatch:
  1. pull remote to catch up
  2. retry push from new base
- If local file missing: re‑create and pull from offset 0.

### Notes
- The daemon does **not** sync `.fray/local/` runtime data.
- Presence remains a separate realtime service.

---

## 3) Migration Path for Existing Git‑Synced Channels

Goal: move an existing git‑synced `.fray/shared/` into hosted sync **without breaking** the event model.

### Assumptions
- Git‑synced `.fray/shared/` is the canonical history.
- Hosted backend will ingest existing streams as‑is.

### Steps

**A) Prepare**
1. Ensure all machines are merged via git:
   - `git pull --rebase` on each machine
   - resolve conflicts
2. Pick a single machine to perform the hosted import.

**B) Create hosted channel**
1. `fray sync setup --hosted <url>`
2. Register machine ID with the hosted service.

**C) Import existing streams**
1. The sync daemon uploads all current JSONL streams.
2. Server stores them as canonical history (no re‑ordering).
3. Server computes stream metadata (line counts, sha256, last_seq).

**D) Switch clients**
1. On each machine:
   - run `fray sync setup --hosted <url>`
   - start sync daemon
2. Verify `fray machines` shows expected remote machines.

**E) Optional: freeze git sync**
- Keep git as **read‑only** archive, or
- Remove `.fray/shared/` from git to avoid dual‑source writes.

### Safety Checks
During import:
- Validate JSONL lines are valid JSON.
- Compute per‑stream checksum and line counts.
- Reject if machine IDs collide at the hosted layer.

### Rollback
If hosted sync is disabled:
- Stop daemon
- Continue git sync (no data loss; files unchanged)

---

## Deliverables Summary

- Query DB schema for messages/threads/questions (above).
- Reference sync daemon behavior + state format.
- Git‑to‑hosted migration steps.

---

## Decision Log (MVP)

1) **Seq validation**
   - Decision: accept seq but do not hard‑enforce monotonicity on the server (MVP).

2) **Web machine identity**
   - Decision: a **portable per‑user virtual machine** (e.g., `web-<user>`).

3) **Web mention encoding**
   - Decision: web UI **requires explicit `@agent@machine`**. Promote last‑used target in autocomplete.

4) **Human display**
   - Decision: **suppress machine suffix** for human users; show suffix for agents when needed.

5) **Merge ordering**
   - Decision: order by **(ts, machine_id, seq/index)**; no server‑assigned global order.

6) **Integrity**
   - Decision: **checksums only** for MVP; plan for AAP signatures later.

7) **Pruning / retention**
   - Decision: **visibility‑only pruning** (archive/hidden by default), never hard delete; opt‑in policy.

8) **Search backend**
   - Decision: **Postgres + FTS** for hosted read model.

---

## Minimal API Contract (Draft)

### Auth
- JWT bearer tokens scoped to `channel_id` + role.
- Short TTL with refresh.

### Register Machine
```
POST /v1/sync/register-machine
{ channel_id, machine_id, device_info }
→ { ok, machine_id, token }
```

### Manifest
```
GET /v1/sync/manifest?channel_id=...
→ { streams: [{machine_id, file, line_count, sha256, last_seq, updated_at}] }
```

### Pull
```
POST /v1/sync/pull
{ channel_id, cursors: [{machine_id, file, line_offset}] }
→ { updates: [{machine_id, file, lines: [...], new_offset}] }
```

### Push
```
POST /v1/sync/push
{
  channel_id,
  machine_id,
  file,
  base: { line_count, sha256, last_seq },
  lines: ["{...}", "{...}"],
  idempotency_key
}
→ { ok, new_line_count, new_sha256, new_last_seq }
```

### Errors (examples)
- `409 base_mismatch` → client should pull then retry push
- `400 invalid_line` → client must retry with corrected batch
- `409 machine_id_conflict`

---

## Threat Model & Integrity Scope (MVP)

**Checksums only** protect against:
- Accidental corruption
- Partial sync / truncated lines

Checksums do **not** protect against:
- Malicious modification of shared data
- Forged events

**Future**: AAP signatures for per‑event non‑repudiation.

---

## Edge Cases Checklist

- Seq mismatch between client and server (accept, but log).
- Base mismatch on push → pull + retry.
- Missing local stream file → recreate and pull from offset 0.
- Truncated JSONL line → ignore on read, log warning.
- Clock skew → warn if skew > threshold; keep deterministic ordering.
- Machine ID collision on hosted → reject registration.
- Web user without explicit `@agent@machine` → UI blocks send.

---

## Web UX: Explicit Machine Mentions

- Web UI requires full addresses: `@agent@machine`.
- Autocomplete suggests the **last used machine** first.
- If user types `@agent` without machine suffix:
  - show inline error / suggestion list
  - do not send until resolved
