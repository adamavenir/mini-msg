# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0]

### Added
- Questions: wonder/ask/list/view/answer/close workflow with JSONL + SQLite support
- Threads as playlists with message home + curation, subscriptions, and surfacing/backlinks
- `fray reply` for reply chains (renamed from `fray thread`)
- Thread/Question CLI commands (`thread`, `threads`, `surface`, `note`, `notes`, `meta`)
- Chat TUI thread panel with filtering and pseudo-thread question views
- `fray versions` to show message edit history with optional diffs
- Edit events with required reasons plus edited metadata in message output

### Changed
- Messages now include `home`, `references`, and `surface_message` fields
- Room message queries default to `home = "room"`

## [0.3.0] - 2025-12-22

### Added
- Batch agent updates with `fray batch-update`
- Merge command to combine agent history
- Reactions for short replies (<20 chars)
- Chat: input auto-expands up to 8 lines and supports selection/copy
- Chat: click a message to start a threaded reply; double-click to copy
- Chat: shortcut help overlay and clearer layout
- Sidebar: filter channels with `#` or space
- Autocomplete shows agent nicknames
- Roster/info show nicknames and consistent status/purpose fields
- `fray destroy <channel>` to delete a channel entirely
- Prune preserves thread integrity

### Changed
- Chat colors are assigned by recency instead of hash
- Chat: Ctrl-C clears input first, exits only when empty
- Roster uses `here: true|false` instead of `status: active`
- Mention highlighting respects default color

### Fixed
- `fray prune --all` now prunes all messages
- Suggest correct agent name when delimiter differs

## [0.2.0] - 2025-12-19

### Added
- **GUID-based identifiers**: Messages (`msg-xxxx`), agents (`usr-xxxx`), and channels (`ch-xxxx`) now use 8-character base36 GUIDs for stable references across machines
- **JSONL storage**: Append-only `messages.jsonl` and `agents.jsonl` files are the source of truth; edits/deletes append `message_update` records; SQLite is a rebuildable cache
- **Channel system**: Projects are registered as channels with `fray init`, enabling cross-channel operations
- **Cross-channel operations**: `--in <channel>` flag and `fray chat <channel>` for working across projects
- **Time-based queries**: `--since` and `--before` flags accept relative times (`1h`, `2d`), absolute times (`today`, `yesterday`), or GUID prefixes (`#abc`)
- **Reply syntax in chat**: Type `#abc hello` to reply to a message; displays show `#xxxx/#xxxxx/#xxxxxx` suffixes
- **New commands**:
  - `fray ls` - list registered channels
  - `fray history <agent>` - show agent's message history with time filtering
  - `fray between <a> <b>` - show messages between two agents
  - `fray nick <agent> --as <nick>` - add nickname for agent in this channel
  - `fray nicks <agent>` - show agent's nicknames
  - `fray whoami` - show your identity and nicknames
  - `fray prune` - archive old messages with git guardrails
  - `fray edit`, `fray rm`, `fray rename`, `fray view`, `fray filter` - message and agent utilities
  - `fray quickstart`, `fray info`, `fray roster`, `fray config` - onboarding and inspection helpers
  - `fray hook-install`, `fray hook-session`, `fray hook-prompt`, `fray hook-precommit` - Claude Code integration hooks
  - `fray migrate` - migrate v0.1.0 projects to GUID format
- **JSON output**: Most read commands support `--json` flag for programmatic access
- **Cold storage**: `fray prune` moves old messages to `history.jsonl`, requires clean git state

### Changed
- Message IDs changed from numeric to GUID format (`msg-xxxxxxxx`)
- Message references now use GUID prefix matching (`#abc` resolves to full GUID)
- Threading now uses GUID references instead of numeric IDs
- Storage structure: `.fray/` now contains `fray-config.json`, `messages.jsonl`, `agents.jsonl`, and SQLite cache files
- Global config at `~/.config/fray/fray-config.json` tracks registered channels

### Migration
- Run `fray migrate` to convert v0.1.0 projects to v0.2.0 format
- Backup created at `.fray.bak/` before migration
- Messages processed in timestamp order during migration

## [0.1.0] - 2024-12-17

### Added
- Initial release
- Agent registration and messaging system
- @mention routing with prefix matching
- Threading support for conversations
- Read state tracking for messages
- Interactive chat mode for human users
- Claims system for file/issue coordination
- Git pre-commit hook for claim conflict detection
- Claude Code integration via hooks (SessionStart, UserPromptSubmit)
- Claude Desktop MCP server integration
- Message filtering and display customization
- Agent lifecycle commands (new, back, bye)
- Simple agent names (alice, bob) with auto-generated options

[0.3.0]: https://github.com/adamavenir/fray/releases/tag/v0.3.0
[0.2.0]: https://github.com/adamavenir/fray/releases/tag/v0.2.0
[0.1.0]: https://github.com/adamavenir/fray/releases/tag/v0.1.0
