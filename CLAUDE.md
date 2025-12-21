# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**mm** (mini-msg) is a standalone CLI tool for agent-to-agent messaging. It uses GUID-based identifiers with JSONL append-only storage (source of truth) and SQLite as a rebuildable cache. Projects are registered as channels, enabling cross-channel operations.
MCP integration is not yet ported to Go.

**Module**: `github.com/adamavenir/mini-msg`
**Repository**: github.com/adamavenir/mini-msg
**CLI command**: `mm`

## Build Commands

```bash
go build ./cmd/mm  # Build
go test ./...      # Run tests
```

## Architecture

```
cmd/mm/           # CLI entry point
internal/command/ # Cobra commands and helpers
internal/chat/    # Bubble Tea chat UI + highlighting
internal/db/      # SQL schema, queries, JSONL storage/rebuild
internal/core/    # Project discovery, GUIDs, mentions, time parsing
internal/types/   # Go types
```

## Storage Structure

```
.mm/
  mm-config.json      # Project config (channel_id, known_agents, nicks)
  messages.jsonl      # Append-only message log (source of truth)
  agents.jsonl        # Append-only agent log (source of truth)
  history.jsonl       # Archived messages (from mm prune)
  mm.db               # SQLite cache (rebuildable from JSONL)

~/.config/mm/
  mm-config.json      # Global channel registry
```

## Key Patterns

**GUIDs**: All entities use 8-character lowercase alphanumeric GUIDs with prefixes:
- Messages: `msg-a1b2c3d4`
- Agents: `usr-x9y8z7w6`
- Channels: `ch-mmdev12`

**JSONL Storage**: Append-only `messages.jsonl` and `agents.jsonl` are the source of truth. SQLite is a rebuildable cache. Use `rebuildDatabaseFromJsonl()` to reconstruct.

**Agent IDs**: Names like `alice`, `eager-beaver`, `alice.frontend`. Names must start with a lowercase letter and can contain lowercase letters, numbers, hyphens, and dots (e.g., `alice`, `frontend-dev`, `alice.frontend`, `pm.3.sub`). Use `mm new <name>` to register, or `mm new` for random name generation.

**@mentions**: Extracted on message creation, stored as JSON array. Prefix matching using `.` as separator: `@alice` matches `alice`, `alice.frontend`, `alice.1`. The `@all` mention is a broadcast.

**Threading**: Messages can reply to other messages via `reply_to` field (GUID). Use `--reply-to <guid>` when posting. In chat, prefix matching is supported: type `#abc hello` to reply (resolves to full GUID). View threads with `mm thread <guid>`.

**Message types**: Messages have a `type` field: `'agent'` (default) or `'user'`. User messages come from `mm chat`.

**Database**: Uses `better-sqlite3` (synchronous). Tables are prefixed `mm_`. Primary keys are GUIDs (`guid TEXT PRIMARY KEY`).

**Channel Context**: Resolution priority:
1. `--in <channel>` flag (explicit)
2. Current directory discovery (if .mm/ exists)

**Time Queries**: `parseTimeExpression()` handles relative (`1h`, `2d`), absolute (`today`, `yesterday`), and GUID prefix (`#abc`) formats.

**Project discovery**: `discoverProject()` walks up from cwd looking for `.mm/` directory. Initialize with `mm init`. Running `mm chat` in an uninitialized directory prompts to init.

## Claims System

Claims prevent agents from accidentally working on the same files, issues, or beads tickets. When an agent claims a resource, other agents see a warning if they try to commit files matching those patterns.

**Claim types:**
- `file` - file paths or glob patterns (e.g., `src/auth.ts`, `lib/*.ts`)
- `bd` - beads issue IDs
- `issue` - GitHub issue numbers

**Commands:**
```bash
mm claim @alice --file src/auth.ts --bd xyz-123    # Claim resources
mm status @alice "fixing auth" --file src/auth.ts  # Goal + claims in one
mm claims                                           # List all claims
mm claims @alice                                    # List agent's claims
mm clear @alice                                     # Clear all claims
mm clear @alice --file src/auth.ts                  # Clear specific claim
```

**Pre-commit hook:**
```bash
mm hook-install --precommit    # Install git pre-commit hook
```
The hook warns when committing files claimed by other agents. Advisory by default; use `mm config precommit_strict true` for blocking mode.

## Claude Code Hooks

mm integrates with Claude Code via hooks for ambient chat awareness:

```bash
mm hook-install   # Install hooks to .claude/settings.local.json
```

This installs:
- **SessionStart**: Prompts unregistered agents to join, or injects room context
- **UserPromptSubmit**: Injects latest messages before each prompt

Agents register via `mm new <name>`, which auto-writes `MM_AGENT_ID` to `CLAUDE_ENV_FILE` when running under hooks.

## Migration

**From v0.1.0 to v0.2.0:**
```bash
mm migrate            # Migrate to GUID-based format
```
This creates a backup at `.mm.bak/`, generates GUIDs for messages and agents (processed in timestamp order), creates JSONL files, and registers the channel in the global config.

## Testing

Tests create temporary mm projects using `mm init` in isolated temp directories.

## Quick Reference

```bash
# Initialize
mm init                      # Create .mm/ in current directory
mm chat                      # Auto-prompts to init if needed

# Agent lifecycle
mm new alice "message"       # Register as alice and post join message
mm new                       # Generate random name like "eager-beaver"
mm here                      # Who's active (with claim counts)
mm bye alice "message"       # Leave (auto-clears claims)
mm whoami                    # Show your identity and nicknames

# Messaging
mm post --as alice "message" # Post (use @mentions)
mm post --as alice -r <guid> # Reply to message
mm get alice                 # Room + my @mentions
mm @alice                    # Check mentions for alice
mm thread <guid>             # View message thread
mm history alice             # Show agent's message history
mm between alice bob         # Messages between two agents

# Time-based queries
mm get --since 1h            # Last hour
mm get --since today         # Since midnight
mm get --since #abc          # After specific message
mm history alice --since 2d  # Last 2 days

# Channels
mm ls                        # List registered channels
mm chat <channel>            # Chat in specific channel
mm --in <channel> ...        # Operate in another channel

# Nicknames
mm nick @alice --as helper   # Add nickname
mm nicks @alice              # Show nicknames

# Claims (collision prevention)
mm claim @alice --file path      # Claim a file
mm claim @alice --file "*.ts"    # Claim glob pattern
mm claim @alice --bd xyz-123     # Claim beads issue
mm claim @alice --issue 456      # Claim GitHub issue
mm status @alice "msg" --file x  # Update goal + claim
mm status @alice --clear         # Clear goal + claims
mm claims                        # List all claims
mm claims @alice                 # List agent's claims
mm clear @alice                  # Clear all claims
mm clear @alice --file path      # Clear specific claim

# For humans
mm chat                      # Interactive chat mode
mm watch                     # Tail messages
mm prune                     # Archive old messages

# JSON output
mm get --last 10 --json      # Any read command supports --json

# Migration
mm migrate                   # Migrate from v0.1.0 to v0.2.0
```
