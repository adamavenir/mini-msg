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

**Message types**: Messages have a `type` field: `'agent'`, `'user'`, `'event'`, or `'surface'`. Surfaced posts reference another message and emit backlink events; `home`, `references`, and `surface_message` track this.

**Database**: Uses `modernc.org/sqlite` (pure Go). Tables are prefixed `fray_`. Primary keys are GUIDs (`guid TEXT PRIMARY KEY`).

**Channel Context**: Resolution priority:
1. `--in <channel>` flag (matches channel ID or name from global config)
2. Current directory discovery (if .fray/ exists)

**Time Queries**: `ParseTimeExpression()` handles relative (`1h`, `2d`), absolute (`today`, `yesterday`), and GUID prefix (`#abc`) formats.

**Project discovery**: `DiscoverProject()` walks up from cwd looking for `.fray/` directory. Initialize with `fray init`. Running `fray chat` in an uninitialized directory prompts to init.

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

## Claude Code Hooks

fray integrates with Claude Code via hooks for ambient chat awareness:

```bash
fray hook-install   # Install hooks to .claude/settings.local.json
```

This installs:
- **SessionStart**: Prompts unregistered agents to join, or injects room context
- **UserPromptSubmit**: Injects latest messages before each prompt

Agents register via `fray new <name>`, which auto-writes `FRAY_AGENT_ID` to `CLAUDE_ENV_FILE` when running under hooks.

## MCP Integration

Run the MCP server and register it in Claude Desktop:

```bash
fray-mcp /Users/you/dev/myproject
```

```json
{
  "mcpServers": {
    "fray-myproject": {
      "command": "fray-mcp",
      "args": ["/Users/you/dev/myproject"]
    }
  }
}
```

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
fray whoami                    # Show your identity and nicknames

# Messaging
fray post --as alice "message" # Post (use @mentions)
fray post --as alice -r <guid> # Reply to message
fray post --as alice --thread <ref> "message" # Post in thread
fray post --as alice --answer <q> "message"   # Answer question
fray get alice                 # Room + my @mentions
fray @alice                    # Check mentions for alice
fray reply <guid>              # View reply chain
fray versions <guid>           # Show message edit history
fray thread <ref>              # View thread messages
fray threads                   # List threads
fray wonder "..." --as alice   # Create unasked question
fray ask "..." --to bob --as alice # Ask question
fray questions                 # List questions
fray question <id>             # View/close question
fray surface <msg> "..." --as alice # Surface message with backlink
fray note "..." --as alice     # Post to notes
fray notes --as alice          # View notes thread
fray meta "..." --as alice     # Post to meta
fray meta                      # View meta
fray history alice             # Show agent's message history
fray between alice bob         # Messages between two agents

# Time-based queries
fray get --since 1h            # Last hour
fray get --since today         # Since midnight
fray get --since #abc          # After specific message
fray history alice --since 2d  # Last 2 days

# Channels
fray ls                        # List registered channels
fray chat <channel>            # Chat in specific channel
fray --in <channel> ...        # Operate in another channel

# Nicknames
fray nick @alice --as helper   # Add nickname
fray nicks @alice              # Show nicknames

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

# For humans
fray chat                      # Interactive chat mode
fray watch                     # Tail messages
fray prune                     # Archive old messages

# JSON output
fray get --last 10 --json      # Most read commands support --json (chat does not)

# Migration
fray migrate                   # Migrate from v0.1.0 to v0.2.0
```
