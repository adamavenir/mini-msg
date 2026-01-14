# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Fray** is a standalone CLI tool for agent-to-agent messaging. It uses GUID-based identifiers with JSONL append-only storage (source of truth) and SQLite as a rebuildable cache. Projects are registered as channels, enabling cross-channel operations.
MCP integration is available via the `fray-mcp` binary.

**Module**: `github.com/adamavenir/fray`
**Repository**: github.com/adamavenir/fray
**CLI command**: `fray`

## Build Commands

```bash
go build ./cmd/fray      # Build
go build ./cmd/fray-mcp  # Build MCP server
go test ./...          # Run tests
```

## Architecture

```
cmd/fray/           # CLI entry point
internal/command/ # Cobra commands and helpers
internal/chat/    # Bubble Tea chat UI + highlighting
internal/db/      # SQL schema, queries, JSONL storage/rebuild
internal/core/    # Project discovery, GUIDs, mentions, time parsing
internal/types/   # Go types
```

## Storage Structure

```
.fray/
  fray-config.json      # Project config (channel_id, known_agents, nicks)
  messages.jsonl      # Append-only message log (source of truth)
  agents.jsonl        # Append-only agent log (source of truth)
  questions.jsonl     # Append-only question log (source of truth)
  threads.jsonl       # Append-only thread + event log (source of truth)
  history.jsonl       # Archived messages (from fray prune)
  .gitignore          # Ignores *.db files
  fray.db               # SQLite cache (rebuildable from JSONL)
  fray.db-wal           # SQLite write-ahead log (gitignored)
  fray.db-shm           # SQLite shared memory (gitignored)

~/.config/fray/
  fray-config.json      # Global channel registry
```

## Key Patterns

**GUIDs**: All entities use 8-character base36 GUIDs with prefixes:
- Messages: `msg-a1b2c3d4`
- Agents: `usr-x9y8z7w6`
- Channels: `ch-fraydev12`

**JSONL Storage**: Append-only `messages.jsonl` and `agents.jsonl` are the source of truth. Edits/deletes append `message_update` records. SQLite is a rebuildable cache. Use `RebuildDatabaseFromJSONL()` to reconstruct.

**Agent IDs**: Names like `alice`, `eager-beaver`, `alice.frontend`. Names must start with a lowercase letter and can contain lowercase letters, numbers, hyphens, and dots (e.g., `alice`, `frontend-dev`, `alice.frontend`, `pm.3.sub`). Use `fray new <name>` to register, or `fray new` for random name generation.

**@mentions**: Extracted on message creation, stored as JSON array. Prefix matching using `.` as separator: `@alice` matches `alice`, `alice.frontend`, `alice.1`. The `@all` mention is a broadcast.

**Threading**: Messages can reply to other messages via `reply_to` field (GUID). Use `--reply-to <guid>` when posting. In chat, prefix matching is supported: type `#abc hello` to reply (resolves to full GUID). View reply chains with `fray reply <guid>`. Container threads are playlists: messages have a `home` (room or thread) and can be curated into multiple threads.

**Thread Curation**: Threads support:
- **Anchors**: A designated message serving as TL;DR, shown at top of thread display
- **Pins**: Messages can be pinned within threads for easy reference (per-thread, not global)
- **Moving**: Use `fray mv` to move messages between threads/room, or reparent threads under different parents
- **Activity tracking**: `last_activity_at` tracks when messages are added/moved

**Thread Types**: Threads have a `type` field:
- `standard` - normal user-created threads
- `knowledge` - knowledge hierarchy threads (auto-created for agents/roles)
- `system` - system-managed threads (notes, keys, jrnl)

**Knowledge Hierarchy**: Agents and roles have dedicated thread hierarchies:
```
meta/                    # Root knowledge thread
├── {agent}/             # Agent's root thread (knowledge type)
│   ├── notes/           # Working notes (ephemeral)
│   └── jrnl/            # Personal journal

roles/{role}/            # Role's root thread (knowledge type)
├── meta/                # Role-specific context
└── keys/                # Atomic insights for the role
```
- Agent hierarchies auto-created on `fray new`, `fray back`, and `fray agent create`
- Role hierarchies auto-created on `fray role add/play`
- Use `fray get meta/{agent}/notes` for agent notes, `fray get meta` for project-wide context
- Use `fray post roles/<role>/keys` to record role insights, `fray get roles/<role>/keys` to view

**Message types**: Messages have a `type` field: `'agent'`, `'user'`, `'event'`, or `'surface'`. Surfaced posts reference another message and emit backlink events; `home`, `references`, and `surface_message` track this.

**Database**: Uses `modernc.org/sqlite` (pure Go). Tables are prefixed `fray_`. Primary keys are GUIDs (`guid TEXT PRIMARY KEY`).

**Channel Context**: Resolution priority:
1. `--in <channel>` flag (matches channel ID or name from global config)
2. Current directory discovery (if .fray/ exists)

**Time Queries**: `ParseTimeExpression()` handles relative (`1h`, `2d`), absolute (`today`, `yesterday`), and GUID prefix (`#abc`) formats.

**Project discovery**: `DiscoverProject()` walks up from cwd looking for `.fray/` directory. Initialize with `fray init`. Running `fray chat` in an uninitialized directory prompts to init.

## Managed Agents (Daemon Support)

Agents can be daemon-managed, enabling automatic spawning on @mentions.

**Agent fields:**
- `managed: bool` - whether daemon controls this agent
- `invoke.driver` - CLI driver: `claude`, `codex`, `opencode`
- `invoke.model` - model to use (e.g., `sonnet-1m` for 1M context Sonnet)
- `invoke.trust` - trust capabilities: `["wake"]` allows agent to trigger spawns for other agents
- `invoke.prompt_delivery` - how prompts are passed: `args`, `stdin`, `tempfile`
- `invoke.spawn_timeout_ms` - max time in 'spawning' state (default: 30000)
- `invoke.idle_after_ms` - time since activity before 'idle' (default: 5000)
- `invoke.min_checkin_ms` - done-detection: idle + no fray posts = kill (default: 0 = disabled)
- `invoke.max_runtime_ms` - zombie safety net: forced termination (default: 0 = unlimited)
- `presence` - daemon-tracked state: `active`, `spawning`, `idle`, `error`, `offline`
- `mention_watermark` - last processed msg_id for debouncing

**Session lifecycle:** Sessions run until the agent exits (`fray bye`, `fray brb`, or `/land`). An @mention restarts a stopped session.
- `min_checkin_ms`: Optional auto-kill if idle with no fray activity (disabled by default)
- `max_runtime_ms`: Optional hard time limit (disabled by default)
- Use `fray heartbeat` to signal activity during long-running work without posting
- Use `fray brb` for seamless handoff - daemon immediately spawns fresh session with continuation prompt

**Session events** (stored in `agents.jsonl`):
- `session_start`: agent spawned (includes `triggered_by` msg_id)
- `session_end`: session completed (includes `exit_code`, `duration_ms`)
- `session_heartbeat`: periodic health updates

**JSONL record types:**
- `agent` - agent registration
- `agent_update` - partial updates (status, presence, watermark)
- `session_start`, `session_end`, `session_heartbeat` - session lifecycle

## Jobs (Parallel Agent Coordination)

Jobs enable parallel execution of multiple agent workers on a single task. Workers are ephemeral agents with bracket-notation IDs like `dev[abc1-0]`.

**Worker ID format:** `base[suffix-idx]`
- `base` - the base agent type (e.g., `dev`, `reconciler`)
- `suffix` - first 4 chars after "job-" prefix (e.g., job-abc12345 → `abc1`)
- `idx` - worker index (0-based, auto-assigned or explicit)

Example: `dev[abc1-0]`, `pm.frontend[xyz9-3]`

**Job commands:**
```bash
fray job create "task name" --as pm                    # Create job, returns job-xxx GUID
fray job create "task" --context '{"issues":["id"]}'   # With context (issues/threads/messages)
fray job join job-abc12345 --as dev                    # Join (auto-index)
fray job join job-abc12345 --as dev --idx 2            # Join (explicit index)
```

**Job lifecycle:**
1. Create job: generates GUID, creates associated thread (thread name = job GUID)
2. Workers join: creates ephemeral agent with JobID/JobIdx fields
3. Workers do work (post to job thread or room)
4. Workers leave: `fray bye` (ephemeral agent stays but goes offline)
5. Close job: `fray job close <job-id>` (default: completed; use --status for cancelled/failed)

**Database fields (types.Agent):**
- `job_id` - FK to fray_jobs.guid (nil for regular agents)
- `job_idx` - worker index within job (0-based)
- `is_ephemeral` - true for job workers

**Ambiguous mention handling:**
- `db.IsAmbiguousMention(db, "dev")` returns true if dev has active job workers
- When ambiguous, bare `@dev` mentions should be blocked or warn
- Daemon skips spawning base agents with active workers
- Use specific worker IDs (`@dev[abc1-0]`) to target workers

**External orchestration:**
Jobs are designed for external coordination via mlld scripts. The daemon tracks presence but doesn't manage job worker spawning.

## Claims System

Claims prevent agents from accidentally working on the same files, issues, or beads tickets. When an agent claims a resource, other agents see a warning if they try to commit files matching those patterns.

**Claim types:**
- `file` - file paths or glob patterns (e.g., `src/auth.ts`, `lib/*.ts`)
- `bd` - beads issue IDs
- `issue` - GitHub issue numbers

**Commands:**
```bash
fray claim @alice --file src/auth.ts --bd xyz-123    # Claim resources
fray status @alice "fixing auth" --file src/auth.ts  # Goal + claims in one
fray claims                                           # List all claims
fray claims @alice                                    # List agent's claims
fray clear @alice                                     # Clear all claims
fray clear @alice --file src/auth.ts                  # Clear specific claim
```

**Pre-commit hook:**
```bash
fray hook-install --precommit    # Install git pre-commit hook
```
The hook warns when committing files claimed by other agents. Advisory by default; use `fray config precommit_strict true` for blocking mode.

## Beads Issue Tracking

Beads (`bd`) tracks work items with dependencies. Issues flow through states:
`blocked → ready → in_progress → closed`

**Dependency graph**: Issues can depend on other issues. `bd ready` shows only unblocked work.

```bash
bd create "fix auth bug" --type bug     # Create issue
bd ready                                 # Show unblocked issues
bd deps add fray-abc fray-xyz           # fray-abc depends on fray-xyz
bd update fray-abc --status in_progress # Start work
bd close fray-abc --reason "..."        # Complete with reason
```

**Work on `bd ready` issues** unless explicitly asked otherwise. This ensures you're not blocked.

## Claude Code Hooks

fray integrates with Claude Code via hooks for ambient chat awareness:

```bash
fray hook-install            # Install integration hooks
fray hook-install --safety   # Also install safety guards
fray hook-install --safety --global  # Install safety globally (~/.claude)
```

**Integration hooks** (installed by default):
- **SessionStart**: Prompts unregistered agents to join, or injects room context
- **UserPromptSubmit**: Injects latest messages before each prompt
- **PreCompact**: Reminds to preserve work before context compaction
- **SessionEnd**: Records session end for presence tracking

**Safety guards** (installed with `--safety`):
Protects `.fray/` data from destructive git commands:
- `git stash` blocked when `.fray/` has uncommitted changes
- `git checkout/restore <files>` blocked when `.fray/` dirty
- `git reset --hard` blocked when `.fray/` dirty
- `git clean -f` always blocked (deletes untracked files)
- `rm .fray/` or `rm .fray/*.jsonl` always blocked
- `git push --force` to main/master always blocked

Safe operations still allowed: `git checkout -b`, `git restore --staged`, `git clean -n`, `git stash pop/apply/list`.

Agents register via `fray new <name>`, which auto-writes `FRAY_AGENT_ID` to `CLAUDE_ENV_FILE` when running under hooks.

## MCP Integration

Run the MCP server and register it in Claude Desktop:

```bash
fray-mcp /path/to/project [agent-name]
```

```json
{
  "mcpServers": {
    "fray-myproject": {
      "command": "/path/to/fray-mcp",
      "args": ["/Users/you/dev/myproject", "claude-desktop"]
    }
  }
}
```

The agent name is optional (default: `desktop`). Provides two tools: `fray_post` (auto-joins on first post) and `fray_get`.

## Migration

**From v0.1.0 to v0.2.0:**
```bash
fray migrate            # Migrate to GUID-based format
```
This creates a backup at `.fray.bak/`, generates GUIDs for messages and agents (processed in timestamp order), creates JSONL files, and registers the channel in the global config.

## Testing

Tests create temporary fray projects using `fray init` in isolated temp directories.

## Quick Reference

```bash
# Initialize
fray init                      # Create .fray/ in current directory
fray chat                      # Auto-prompts to init if needed

# Agent lifecycle
fray new alice "message"       # Register as alice and post join message
fray new                       # Generate random name like "eager-beaver"
fray here                      # Who's active (with claim counts)
fray bye alice "message"       # Leave (auto-clears claims)
fray brb alice "message"       # Hand off to fresh session (daemon spawns immediately)
fray whoami                    # Show your identity and nicknames

# Messaging (path-based)
fray post "message" --as alice         # Post to room
fray post meta "message" --as alice    # Post to project meta
fray post opus/notes "msg" --as alice  # Post to agent notes path
fray post design-thread "msg" --as a   # Post to named thread
fray post -r <guid> "reply" --as alice # Reply to message
fray get                               # Room + notifs (uses FRAY_AGENT_ID)
fray get --as opus                     # Room + notifs for agent
fray get meta                          # View project meta
fray get opus/notes                    # View agent notes path
fray get design-thread                 # View thread by name
fray get design-thread --pinned        # Pinned messages only
fray get design-thread --by @alice     # Messages from agent
fray get design-thread --with "text"   # Messages containing text
fray get design-thread --reactions     # Messages with reactions
fray get notifs --as opus              # Notifications only
fray msg-abc123                        # View specific message (shorthand)
fray @alice                            # Check mentions for alice

# Thread operations (path-based)
fray thread design-thread              # View or create thread
fray thread opus/notes "Summary"       # Create nested thread with anchor
fray threads                           # List threads
fray threads --following               # List threads you follow
fray threads --activity                # Sort by recent activity
fray threads --pinned                  # List pinned threads only
fray threads --muted                   # List muted threads only
fray threads --all                     # Include muted threads
fray threads --tree                    # Show as tree with indicators
fray follow design-thread --as alice   # Follow/subscribe to thread
fray unfollow design-thread --as alice # Unfollow thread
fray mute design-thread --as alice     # Mute thread notifications
fray unmute design-thread --as alice   # Unmute thread
fray add design-thread msg-abc         # Add message to thread
fray remove design-thread msg-abc      # Remove from thread
fray anchor design-thread msg-abc      # Set thread anchor
fray archive design-thread             # Archive thread
fray restore design-thread             # Restore archived thread
fray thread rename <thread> <name>     # Rename a thread
fray pin <msg> [--thread <ref>]        # Pin message in thread
fray unpin <msg> [--thread <ref>]      # Unpin message
fray mv <msg...> <dest>                # Move messages to thread/room
fray mv <msg> main                     # Move message back to room (also: room, channel-name)
fray mv <thread> <parent>              # Reparent thread under another thread
fray mv <thread> <parent> "anchor"     # Reparent + set anchor message
fray mv <thread> root                  # Make thread root-level (also: /)

# Universal operations (by ID prefix)
fray rm msg-abc123                     # Delete message
fray rm thrd-xyz789                    # Delete (archive) thread
fray fave msg-abc --as alice           # Fave message
fray fave thrd-xyz --as alice          # Fave thread (also subscribes)

# Message editing
fray edit <msgid> "new text" --as a    # Edit a message
fray edit <msgid> "text" -m "reason"   # Edit with reason
fray versions <msgid>                  # Show edit history

# Reactions & Surfacing
fray react <emoji> <msg> --as alice    # Add reaction to message
fray surface <msg> "comment" --as a    # Surface message to room with backlink

# Questions
fray wonder "..." --as alice           # Create unasked question
fray ask "..." --to bob --as alice     # Ask question
fray questions                         # List questions
fray question <id>                     # View/close question
fray post --answer <q> "answer" --as a # Answer question

# Knowledge hierarchy (via path-based commands)
fray post opus/notes "..." --as opus   # Post to agent notes
fray get opus/notes                    # View agent notes
fray post meta "..." --as opus         # Post to project meta
fray get meta                          # View project meta
fray post roles/architect/keys "..."   # Record role key insight
fray get roles/architect/keys          # View architect keys

# Legacy (still works)
fray thread subscribe <ref> --as a     # Old subscribe syntax
fray thread unsubscribe <ref> --as a   # Old unsubscribe syntax
fray get alice                         # Agent-based room + mentions

# Time-based queries
fray get --since 1h --as opus          # Last hour
fray get --since today --as opus       # Since midnight
fray get --since #abc --as opus        # After specific message

# Channels
fray ls                                # List registered channels
fray chat <channel>                    # Chat in specific channel
fray --in <channel> ...                # Operate in another channel

# Nicknames
fray nick @alice --as helper           # Add nickname
fray nicks @alice                      # Show nicknames

# Faves (personal collections)
fray fave <item> --as alice            # Fave thread or message
fray unfave <item> --as alice          # Remove from faves
fray faves --as alice                  # List all faves
fray faves --as alice --threads        # List only faved threads

# Reactions (cross-thread queries)
fray reactions --by alice              # Messages alice reacted to
fray reactions --to alice              # Reactions on alice's messages

# Claims (collision prevention)
fray claim @alice --file path      # Claim a file
fray claim @alice --file "*.ts"    # Claim glob pattern
fray claim @alice --bd xyz-123     # Claim beads issue
fray claim @alice --issue 456      # Claim GitHub issue
fray status @alice "msg" --file x  # Update goal + claim
fray status @alice --clear         # Clear goal + claims
fray claims                        # List all claims
fray claims @alice                 # List agent's claims
fray clear @alice                  # Clear all claims
fray clear @alice --file path      # Clear specific claim

# Managed agents (daemon-controlled)
fray agent create <name> --driver claude  # Create managed agent config
fray agent list                    # Show agents with presence/driver
fray agent list --managed          # Show only managed agents
fray agent start <name>            # Start fresh session (/fly prompt)
fray agent start <name> --prompt "..." # Start with custom prompt
fray agent refresh <name>          # End current + start new session
fray agent end <name>              # Graceful session end
fray agent check <name>            # Daemon-less poll (for CI/cron)
fray heartbeat --as <name>         # Silent checkin (resets done-detection timer)
fray heartbeat                     # Uses FRAY_AGENT_ID env var
fray clock                         # Ambient status: timer + notification counts

# Daemon
fray daemon                        # Start daemon (watches @mentions)
fray daemon --debug                # Enable debug logging
fray daemon --poll-interval 2s     # Custom poll interval
fray daemon status                 # Check if daemon is running

# Cooldown & Interrupts
# After clean exit, agents have 30s cooldown before re-spawn
# Interrupt syntax bypasses cooldown and can kill running processes:
!@agent                            # Interrupt + resume same session
!!@agent                           # Interrupt + start fresh session
!@agent!                           # Interrupt, don't spawn after
!!@agent!                          # Force end, don't restart

# Wake conditions (agent coordination - requires trust)
fray wake --on @user1 --as pm      # Wake when specific agents post
fray wake --after 30m --as pm      # Wake after time delay
fray wake --pattern "blocked" --as pm  # Wake on regex pattern match
fray wake --pattern "error" --prompt "Wake for real errors" --as pm  # Pattern + haiku assessment
fray wake --prompt "Wake if dev idle >10min" --poll 1m --as pm  # LLM polling (min 1m)
fray wake --in thread-name --as pm # Scope to specific thread
fray wake --persist --as pm        # Condition survives trigger (manual clear)
fray wake --persist-until-bye --as pm  # Auto-clear on bye
fray wake --persist-restore-on-back --as pm  # Pause on bye, resume on back
fray wake list --as pm             # Show active wake conditions for agent
fray wake clear --as pm            # Clear all wake conditions for agent

# Agent status (for LLM polling)
fray agent status                  # JSON output: agents with presence, status, idle_seconds
fray agent status --managed        # Only show managed agents

# Jobs (parallel agent workers)
fray job create "name" --as pm     # Create job, returns job-xxx GUID
fray job create "name" --as pm --context '{"issues":["id"]}'  # With context JSON
fray job join job-abc12345 --as dev           # Join, auto-index (dev[abc1-0])
fray job join job-abc12345 --as dev --idx 2   # Join, explicit index (dev[abc1-2])
fray job close job-abc12345                   # Close job (default: completed)
fray job close job-abc12345 --status cancelled  # Close with status
fray job leave job-abc12345 --as dev          # Leave as worker
fray job list                                 # List all jobs
fray job status job-abc12345                  # Show job details + workers

# Ghost cursors (session handoffs)
fray cursor set <agent> <home> <msg>       # Set ghost cursor for handoff
fray cursor set <agent> <home> <msg> --must-read  # Mark as must-read
fray cursor show <agent>                   # Show ghost cursors for agent
fray cursor clear <agent>                  # Clear all ghost cursors
fray cursor clear <agent> <home>           # Clear cursor for specific home

# mlld scripts
fray run                       # List available scripts
fray run <name>                # Run script from .fray/llm/
fray run fly @opus             # Run with agent payload
fray run <name> --debug        # Show execution metrics

# For humans
fray chat                      # Interactive chat mode
fray watch                     # Tail messages (shows heartbeat timer if FRAY_AGENT_ID set)
fray prune <target>            # Archive old messages (target: main or thread-name)
fray prune <target> --keep 50  # Keep last 50 messages
fray prune <target> --with faves     # Remove protection, allow pruning faved
fray prune <target> --without reacts # Only prune items without reactions

# JSON output
fray get --last 10 --json      # Most read commands support --json (chat does not)

# Maintenance
fray rebuild                   # Rebuild database from JSONL (fixes schema errors)
fray migrate                   # Migrate from v0.1.0 to v0.2.0
fray install-notifier          # Install macOS notification app with fray icon

# Claude Code hooks
fray hook-install              # Install integration hooks (session, prompt, precompact)
fray hook-install --safety     # Also install safety guards (protect .fray/)
fray hook-install --safety --global  # Install safety guards for all projects
fray hook-uninstall --safety   # Remove safety guards

# Permission requests (interactive approval flow)
fray approve <perm-id> <1|2|3> # Approve permission (1=once, 2=session, 3=project)
fray deny <perm-id> [reason]   # Deny permission request
```
