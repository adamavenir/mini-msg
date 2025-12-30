# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0]

### Added
- Questions: wonder/ask/list/view/answer/close workflow with JSONL + SQLite support
- Threads as playlists with message home + curation, subscriptions, and surfacing/backlinks
- `mm reply` for reply chains (renamed from `mm thread`)
- Thread/Question CLI commands (`thread`, `threads`, `surface`, `note`, `notes`, `meta`)
- Chat TUI thread panel with filtering and pseudo-thread question views

### Changed
- Messages now include `home`, `references`, and `surface_message` fields
- Room message queries default to `home = "room"`

## [0.3.0] - 2025-12-22

### Added
- Batch agent updates with `mm batch-update`
- Merge command to combine agent history
- Reactions for short replies (<20 chars)
- Chat: input auto-expands up to 8 lines and supports selection/copy
- Chat: click a message to start a threaded reply; double-click to copy
- Chat: shortcut help overlay and clearer layout
- Sidebar: filter channels with `#` or space
- Autocomplete shows agent nicknames
- Roster/info show nicknames and consistent status/purpose fields
- `mm destroy <channel>` to delete a channel entirely
- Prune preserves thread integrity

### Changed
- Chat colors are assigned by recency instead of hash
- Chat: Ctrl-C clears input first, exits only when empty
- Roster uses `here: true|false` instead of `status: active`
- Mention highlighting respects default color

### Fixed
- `mm prune --all` now prunes all messages
- Suggest correct agent name when delimiter differs

## [0.2.0] - 2025-12-19

### Added
- **GUID-based identifiers**: Messages (`msg-xxxx`), agents (`usr-xxxx`), and channels (`ch-xxxx`) now use 8-character base36 GUIDs for stable references across machines
- **JSONL storage**: Append-only `messages.jsonl` and `agents.jsonl` files are the source of truth; edits/deletes append `message_update` records; SQLite is a rebuildable cache
- **Channel system**: Projects are registered as channels with `mm init`, enabling cross-channel operations
- **Cross-channel operations**: `--in <channel>` flag and `mm chat <channel>` for working across projects
- **Time-based queries**: `--since` and `--before` flags accept relative times (`1h`, `2d`), absolute times (`today`, `yesterday`), or GUID prefixes (`#abc`)
- **Reply syntax in chat**: Type `#abc hello` to reply to a message; displays show `#xxxx/#xxxxx/#xxxxxx` suffixes
- **New commands**:
  - `mm ls` - list registered channels
  - `mm history <agent>` - show agent's message history with time filtering
  - `mm between <a> <b>` - show messages between two agents
  - `mm nick <agent> --as <nick>` - add nickname for agent in this channel
  - `mm nicks <agent>` - show agent's nicknames
  - `mm whoami` - show your identity and nicknames
  - `mm prune` - archive old messages with git guardrails
  - `mm edit`, `mm rm`, `mm rename`, `mm view`, `mm filter` - message and agent utilities
  - `mm quickstart`, `mm info`, `mm roster`, `mm config` - onboarding and inspection helpers
  - `mm hook-install`, `mm hook-session`, `mm hook-prompt`, `mm hook-precommit` - Claude Code integration hooks
  - `mm migrate` - migrate v0.1.0 projects to GUID format
- **JSON output**: Most read commands support `--json` flag for programmatic access
- **Cold storage**: `mm prune` moves old messages to `history.jsonl`, requires clean git state

### Changed
- Message IDs changed from numeric to GUID format (`msg-xxxxxxxx`)
- Message references now use GUID prefix matching (`#abc` resolves to full GUID)
- Threading now uses GUID references instead of numeric IDs
- Storage structure: `.mm/` now contains `mm-config.json`, `messages.jsonl`, `agents.jsonl`, and SQLite cache files
- Global config at `~/.config/mm/mm-config.json` tracks registered channels

### Migration
- Run `mm migrate` to convert v0.1.0 projects to v0.2.0 format
- Backup created at `.mm.bak/` before migration
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

[0.3.0]: https://github.com/adamavenir/mini-msg/releases/tag/v0.3.0
[0.2.0]: https://github.com/adamavenir/mini-msg/releases/tag/v0.2.0
[0.1.0]: https://github.com/adamavenir/mini-msg/releases/tag/v0.1.0
