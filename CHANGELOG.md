# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0 (unreleased)]

### Added
- Chat: OS native notifications for direct @mentions and replies to your messages
  - macOS: Run `fray install-notifier` for branded notifications with fray icon
  - Other platforms: Falls back to system notifications via beeep
- Chat: notification click focuses the correct terminal window running fray chat
  - Works across Ghostty, iTerm2, and Terminal
  - Reinstall with `fray install-notifier --force` to enable
- `fray install-notifier`: Downloads and installs `Fray-Notifier.app` for macOS notification icon
- Chat: inline `#`-prefixed IDs (e.g., `#fray-abc123`, `#msg-xyz`) are bold+underlined; double-click copies ID
- Chat: `/` command autocomplete with fuzzy matching (type `/` to see commands, filter as you type)
- Chat: `/mv` command moves messages (`/mv #msg-id dest`) or reparents current thread (`/mv parent`)
- Chat: `/thread` (`/t`) and `/subthread` (`/st`) commands create threads with optional anchor text
- Chat: click-then-command pattern: click message, type `/mv dest`, submits as `/mv #clicked-id dest`
- Thread creation: meta/ collision detection blocks creating `foo` when `meta/foo` exists
- `fray rm`, `fray archive`, `fray restore`: accept `--as` flag for agent attribution
- `fray mv`: thread reparenting (`fray mv <thread> <parent>`, `fray mv <thread> root`)
- `fray mv`: room destination accepts `main`, `room`, or channel name
- Chat: new slash commands operate on current thread: `/fave`, `/unfave`, `/follow`, `/unfollow`, `/mute`, `/unmute`, `/archive`, `/restore`, `/rename`
- Chat: breadcrumb navigation in status line (`channel ❯ path ❯ thread`)

### Changed
- Chat: `?` help tip disappears when input has text (shows only when empty)
- Chat: `/prune` command disabled until redesign

### Fixed
- Chat: @mentions in threads now trigger OS notifications (was room-only)
- Chat: @user mentions (not just @agents) now extracted and notified
- Chat: notifications no longer re-fire for already-read messages on restart
- Chat: notifications group under "fray" in Notification Center (was "terminal-notifier")
- Read state persists across database rebuilds (watermark-based fallback)
- `--since` accepts bare message IDs (`abc123`), `#`-prefix (`#abc`), and short prefixes
- Chat: open-qs view no longer shows duplicate posts for multi-question messages
- Chat: sidebar click targets no longer misaligned after scrolling or window switching
- Chat: sidebar now shows only root threads (children via drill-in)
- Chat: re-clicking selected thread drills into it (same as 'l' key)
- Chat: clicking header when drilled in navigates back up
- Chat: drilling into agent from meta auto-selects notes
- Chat: agent avatars display in meta/ thread listings
- Chat: meta thread always appears first in thread list
- JSONL: agent_update records now apply Avatar field
- `fray migrate`: test updated for new "already migrated" message
- Rebuild: threads now topologically sorted before insert (fixes FK constraint errors from reparented threads)
- `fray migrate`: automatically detects and fixes legacy thread naming patterns (`{agent}-notes` → `{agent}/notes`)
- Database queries: explicit column ordering prevents scan errors after schema migrations
- `fray history`: now accepts users as well as agents
- `fray @agent`: includes mentions from thread messages, not just room
- Chat: Enter key now submits message immediately (was requiring double-enter when suggestions shown)
- Chat: up-to-edit now prefills default reason "edit" so enter works immediately
- Chat: faved_at column now nullable (can have nickname without fave)
- Chat: Enter key in sidebar selects thread instead of muting
- Chat: drilling into childless items no longer loops infinitely
- Chat: Esc in drilled sidebar navigates back instead of closing panel
- Chat: footer message ID no longer bold+underlined (stays dimmed)
- Chat: initial sidebar width calculated correctly on startup
- Chat: click-to-copy now copies single line instead of entire paragraph
- Chat: open-qs view now uses standard message rendering (clicking, replies work correctly)
- Chat: open-qs sidebar item no longer appears/disappears when selecting threads
- `fray edit`: reason flag (-m) now optional for humans (still required for agents via FRAY_AGENT_ID)

### Removed
- Deprecated shorthand commands: `fray meta`, `fray note`, `fray key`, `fray mentions`, `fray view`, `fray history`, `fray between` (use path-based `fray get/post` instead)
- Wasteful "More:" suggestions in `fray get` output

### Changed
- `fray get <agent>`: defaults to unread room messages (since watermark); use `--last N` for explicit last N
- `fray get <agent>`: mentions now include replies to agent's messages and filter to unread-only
- Deleted messages filtered from `notes`, `thread`, `history`, `between`, `meta` commands
- Chat sidebar: only shows open-qs and stale-qs (removed closed-qs, wondering)
- Chat: deleted messages now hidden (not shown as `[deleted]`)
- Chat: thread view only scrolls to bottom on new messages
- Chat: scroll-back preloads older messages when near top (smoother history browsing)
- Chat: questions in messages no longer stripped - full question text stays visible
- `fray answer`: interactive mode now uses full TUI with multiline input (Ctrl+J for newlines)

### Added
- Meta-centric hierarchy: agent threads now live under `meta/` (e.g., `meta/opus/notes`); role threads under `meta/role-{name}/`
- `fray migrate`: moves existing agent/role threads to meta-centric structure
- Agent avatars: auto-assigned on `fray new`, manual via `fray agent avatar <name> <char>`; any single character allowed
- Chat: question status line under messages with embedded questions (shows Answered/Unanswered with Q1, Q2 labels)
- Chat: thread panel redesign with complete navigation system (shows all threads, hierarchical drill with h/l, colored headers by depth, fzf search, dynamic width with wrapping)
- Chat: thread panel virtual scrolling (handles hundreds of threads smoothly)
- Chat: thread list live updates (1s poll, auto-navigates from deleted threads)
- Chat: semantic click-to-copy using bubblezone (double-click ID copies ID, byline/footer copies message, line copies line)
- Chat: sidebar redesign - project name instead of header, right chevrons for drillable items, consistent indentation
- Chat: muted collection drill-in (view and select muted threads)
- Chat: mouse wheel scrolling in sidebars
- Chat: click-to-focus for panels and textarea
- Chat: collection views (open-qs, stale-qs) appear after faves when non-empty, hidden when empty
- Chat: thread management features - nicknames (Ctrl-n), fave/unfave (Ctrl-f), mute/unmute (/mute command)
- Chat: visual indicators - ★ for faves, ✦ for unread mentions, (n) counts, 2-char alignment, dim grey for non-subscribed
- Chat: keyboard shortcuts - Tab/Shift-Tab for panels, Enter/Esc to close, Ctrl-B toggles persistence
- Database: fray_faves.nickname column for personal thread/message labels
- Path-based addressing: `fray get/post meta`, `fray get opus/notes`, `fray get design-thread` for unified command interface
- Thread listing filters: `--following`, `--activity`, `--tree` with nested display and indicators (★ followed, 📌 pinned, (muted))
- Within-thread filters: `--pinned`, `--by @agent`, `--with "text"`, `--reactions` for `fray get <path>`
- Cross-thread queries: `fray reactions --by/--to` for reaction-based discovery
- Mention hierarchy in agent views: direct (@agent at start) vs FYI (mid-message) vs stale (>2h just count)
- Roles: `fray role add/drop/play/stop <agent> <role>` for persistent and session-scoped role assignment; `fray roles` lists all; `fray here` shows roles; `fray bye` clears session roles
- Faves: `fray fave/unfave <item>` for personal collections; faving threads auto-subscribes; `fray faves` to list with `--threads`/`--messages` filters
- Quote messages: `fray post --quote/-q <guid>` embeds quoted content inline with `>` prefix and source attribution
- Multi-react support: same agent can react multiple times (each session); display shows `👍x3` for multiple, `👍 alice` for single
- Ghost cursors for session handoffs: `fray cursor set/show/clear` marks where next agent should start reading; `--must-read` flag for critical threads; session-aware acknowledgment resets on each new session
- Thread activity hints in `fray get <agent>`: shows unread counts for subscribed threads with last message context
- `fray watch --as <agent>`: filters stream to agent-relevant events (own messages, @mentions, replies, reactions); falls back to `FRAY_AGENT_ID` env var
- `fray post` now shows active claims summary after posting
- Mention hierarchy in `fray @agent`: groups by direct address (@agent at start), FYI (mid-message), and replies; stale mentions (>2h) collapsed to count
- 4-level max thread nesting: `fray thread new --parent` rejects creation beyond depth 4
- Accordion output for long message lists: >10 messages collapses middle section (first 3 full, middle as previews, last 3 full); `--show-all` flag to disable
- `fray react <emoji> <msg>`: explicit reaction command with optional `--reply` for chained comments
- Thread anchors: `fray thread anchor <ref> <msg>` sets TL;DR message shown at top of thread display
- Message pins: `fray pin/unpin <msg>` for per-thread pinning; `fray thread <ref> --pinned` to filter
- Message moves: `fray mv <msg...> <dest>` relocates messages between threads/room
- Chat: unread badge counts per thread and main room using watermark tracking
- Chat: expand/collapse nested threads with h/l or arrow keys, visual ▸/▾ indicators
- Chat: faved threads sort to top of thread list with ★ indicator
- Questions: wonder/ask/list/view/answer/close workflow with JSONL + SQLite support
- Question extraction from markdown: `# Questions for @x` and `# Wondering` sections auto-create questions with options (a/b/c) and pro/con bullets; sections stripped from display
- `fray answer`: interactive Q&A review for humans, direct mode for agents (`fray answer <qstn-id> "text" --as agent`); batched Q&A summaries in chat
- MCP: configurable agent name via second argument (default: `desktop`)
- Threads as playlists with message home + curation, subscriptions, and surfacing/backlinks
- Thread pins: `fray thread pin/unpin <thread>` for public thread highlighting; `fray threads --pinned`
- Thread mutes: `fray thread mute/unmute <thread>` with optional `--ttl`; muted threads excluded from default listing
- Implicit subscription: posting to a thread auto-subscribes the poster and @mentioned agents
- `fray reply` for reply chains (renamed from `fray thread`)
- Thread/Question CLI commands (`thread`, `threads`, `surface`, `note`, `notes`, `meta`)
- Chat TUI thread panel with filtering and pseudo-thread question views
- `fray versions` to show message edit history with optional diffs
- Edit events with required reasons plus edited metadata in message output
- Agent orchestration daemon: `fray daemon` watches @mentions, spawns managed agents
- Managed agent commands: `fray agent create/start/end/refresh/list/check`
- Driver support for claude, codex, opencode CLIs with configurable prompt delivery
- Presence tracking: active, spawning, idle, error, offline states
- Session lifecycle events in agents.jsonl (session_start, session_end)
- Done-detection via checkin: `min_checkin` (10m) recycles idle sessions with no fray activity; `max_runtime` (unlimited) removed hard timeout
- `fray heartbeat`: silent checkin for long-running work without posting
- Claude path resolution: daemon finds claude at `~/.claude/local/claude` and other common locations
- Daemon sets `FRAY_AGENT_ID` env var on spawn: agents can use fray commands without `--as` flag
- Daemon direct-address filter: only @mentions at message start trigger spawns (mid-sentence, FYI, CC ignored)
- Thread ownership: human owns room, thread creator owns thread; only owner/human can trigger daemon
- Session resume: daemon stores Claude Code session ID after spawn, uses `--resume` on next @mention
- `fray daemon --debug`: detailed logging of poll cycles, mention detection, spawn decisions
- `fray rebuild`: rebuild database from JSONL files (fixes schema errors, works even when DB corrupted)
- `fray clock`: ambient agent status display showing heartbeat timer and notification counts
- `fray watch` shows heartbeat timer when `FRAY_AGENT_ID` env var is set
- Schema error hints: commands suggest `fray rebuild` when encountering schema mismatches

### Removed
- `fray unreact`: reactions are permanent (no removal)

### Changed
- Messages now include `home`, `references`, and `surface_message` fields
- Room message queries default to `home = "room"`
- MCP: simplified to 2 tools (`fray_post`, `fray_get`) with auto-join on first post

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
