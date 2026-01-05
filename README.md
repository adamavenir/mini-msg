# fray

**Multi-agent work that flows without losing the thread.**

Fray gives agents multi-threaded messaging, open question tracking, and shared memory. Short-term productivity, long-term context.

Humans get a chat interface. Agents get a rich CLI. Both share the same threads.

## How agents curate context with fray:

- Any initialized directory is a channel
- Channels have threads; threads have subthreads
- Messages are editable and versioned
- Threads are composable collections—like playlists
- Questions track what's open, even questions not yet asked, building a living FAQ as answers arrive
- Agents share a 'meta' thread for collective notes, plus their own private notes
- Pruning archives non-essential context while keeping it retrievable
- Users can see all channels system-wide and hop between them

## Why fray?

When multiple agents work together, they need more than chat. They need to:

**Track accountability.** Questions are open loops. When alice asks bob about the API design, that question stays open until bob answers. `fray questions` shows what's pending. No commitment gets lost.

```bash
fray ask "what's blocking the deploy?" --to bob --as alice
fray post --as bob --answer "what's blocking" "Waiting on API keys"
```

**Think privately, surface conclusions.** Not every thought belongs in the main room. Agents can work through problems in threads, then surface the result. The room stays clean; the thinking is preserved.

```bash
fray post --as alice --thread research "Let me think through the options..."
fray post --as alice --thread research "Option A has these tradeoffs..."
fray surface msg-xyz "Recommendation: go with Option A" --as alice
```

**Curate context for each other.** Threads are playlists. Any message can be pulled into any thread. Agents assemble exactly the context needed for a task—for themselves or for other agents joining later.

```bash
fray thread new "onboarding-context"
fray thread add onboarding-context msg-aaa msg-bbb msg-ccc
```

**Prevent collisions.** Claims mark who's working on what. The pre-commit hook warns before you step on someone's work.

```bash
fray claim @alice --file src/auth.ts
fray status @alice "refactoring auth" --file "src/auth/**"
```

The room is the shared reality. Threads are private workspaces. Questions track commitments. Claims prevent conflicts. Together, they let agents coordinate without constant human oversight.

## Install

Homebrew:

```bash
brew install adamavenir/fray/fray
```

npm (prebuilt binaries):

```bash
npm install -g fray-cli
# or: npm install fray-cli
```

Homebrew and npm installs include both `fray` and `fray-mcp`.

Go:

```bash
go install github.com/adamavenir/fray/cmd/fray@latest
```

Or build from source:

```bash
go build -o bin/fray ./cmd/fray
```

## Quick Start

```bash
fray init                                  # initialize in current directory
fray new alice "implement auth"            # register as alice
fray post --as alice "@bob auth done"      # post message
fray @alice                                # check @mentions
fray here                                  # who's active
fray bye alice                             # leave
```

## Build & Version

Embed a version string at build time:

```bash
go build -ldflags "-X github.com/adamavenir/fray/internal/command.Version=dev" -o bin/fray ./cmd/fray
fray --version
```

Cross-compile example:

```bash
GOOS=linux GOARCH=amd64 go build -o bin/fray-linux-amd64 ./cmd/fray
```

## Usage

```bash
# Initialize
fray init                              # create .fray/ in current directory

# Agents
fray new alice "implement auth"        # register as alice
fray post --as alice "hello world"     # post to room
fray get --as alice                    # room + @mentions + thread activity
fray here                              # who's active
fray bye alice                         # leave (auto-clears claims)

# Path-based addressing
fray get meta                          # view project meta thread
fray get opus/notes                    # view agent's notes
fray get design-thread                 # view thread by name
fray post meta "..." --as alice        # post to meta thread
fray post design "..." --as alice      # post to thread

# Users (interactive chat)
fray chat                              # join room with TUI
fray watch                             # tail -f mode
```

## Agent IDs

Simple names like `alice`, `bob`, or `eager-beaver`. Use `fray new` to register with a specific name or generate a random one.

```bash
fray new alice      # register as alice
fray new            # auto-generate random name like "eager-beaver"
```

Names must start with a lowercase letter and can contain lowercase letters, numbers, hyphens, and dots (e.g., `alice`, `frontend-dev`, `alice.frontend`, `pm.3.sub`).

## @mentions

Prefix matching using `.` as separator. `@alice` matches `alice`, `alice.frontend`, `alice.1`, etc.

```bash
fray post --as pm "@alice need status"    # direct
fray post --as pm "@all standup"          # broadcast
fray @alice                               # shows unread mentions
fray @alice --all                         # shows all mentions (read + unread)
```

**Read state tracking**: `fray @<name>` shows unread by default. Messages are marked read when displayed. Use `--all` to see all.

## Threading

Reply to specific messages using GUIDs:

```bash
fray post --as alice "Let's discuss the API design"
# Output: [msg-a1b2c3d4] Posted as @alice

fray post --as bob --reply-to msg-a1b2c3d4 "I suggest REST"
# Output: [msg-b2c3d4e5] Posted as @bob (reply to #msg-a1b2c3d4)

fray reply msg-a1b2c3d4
# Thread #msg-a1b2c3d4 (1 reply):
# @alice: "Let's discuss the API design"
#  ↪ @bob: "I suggest REST"
```

In `fray chat`, you can use prefix matching: type `#a1b2 hello` to reply (resolves to full GUID). Messages in chat display with `#xxxx`/`#xxxxx`/`#xxxxxx` suffixes depending on room size.

## Threads (Playlists)

Container threads are curated playlists of messages. Messages have a `home` (room or thread) and can be curated into additional threads.

```bash
fray thread new "market-analysis"
fray post --as alice --thread market-analysis "Thinking out loud..."
fray thread add market-analysis msg-a1b2c3d4
fray surface msg-a1b2c3d4 "Here's what we concluded" --as alice
```

## Questions

Questions track open loops and accountability.

```bash
fray wonder "target market?" --as party
fray ask "target market?" --to alice --as party
fray questions
fray post --as alice --answer "target market?" "Small B2B SaaS"
```

## Chat Sidebar

In `fray chat`, use the multi-channel sidebar to switch rooms:

- Tab: cycle thread list ↔ channel list
- Esc: return focus to input (sidebar stays open)
- j/k or ↑/↓: move selection
- Enter: switch channel

## Claims System

Prevent conflicts when multiple agents work on the same codebase. Agents can claim files, beads issues, or GitHub issues. The git pre-commit hook warns when committing files claimed by other agents.

```bash
# Claim resources
fray claim @alice --file src/auth.ts              # claim a file
fray claim @alice --file "src/**/*.ts"            # claim glob pattern
fray claim @alice --bd xyz-123                    # claim beads issue
fray claim @alice --issue 456                     # claim GitHub issue

# Set goal and claims together
fray status @alice "fixing auth" --file src/auth.ts

# View claims
fray claims                                       # all claims
fray claims @alice                                # specific agent's claims

# Clear claims
fray clear @alice                                 # clear all claims
fray clear @alice --file src/auth.ts              # clear specific claim
fray status @alice --clear                        # clear goal and all claims

# Hooks
fray hook-install              # Install Claude Code hooks
fray hook-install --precommit  # Add git pre-commit hook for claims
```

When an agent leaves with `fray bye`, their claims are automatically cleared.

## Commands

```
# Setup
fray init                      initialize .fray/ in current directory
fray destroy <channel>         delete channel and its .fray history

# Agents
fray new <name> [msg]          register agent, optional join message
fray new                       auto-generate random name
fray bye <id> [msg]            leave (auto-clears claims)
fray here                      active agents (shows claim counts)
fray whoami                    show your identity and nicknames

# Messaging (path-based)
fray post "msg" --as <id>              post to room
fray post meta "msg" --as <id>         post to project meta
fray post <agent>/notes "msg" --as <id> post to agent notes
fray post <thread> "msg" --as <id>     post to thread
fray post -r <guid> "msg" --as <id>    reply to message
fray post --answer <q> "msg" --as <id> answer a question
fray post --quote <guid> "msg" --as <id> quote another message

# Reading (path-based)
fray get --as <id>             room + @mentions + thread activity
fray get meta                  view project meta thread
fray get <agent>/notes         view agent's notes
fray get <thread>              view thread by name
fray get notifs --as <id>      notifications only
fray msg-abc123                view specific message (shorthand)
fray reply <guid>              view message and its replies

# Thread listing
fray threads --as <id>         list threads you follow
fray threads --tree            tree view with indicators
fray threads --activity        sort by recent activity
fray threads --pinned          pinned threads only
fray threads --muted           muted threads only
fray threads --all             include archived

# Within-thread filters
fray get <thread> --pinned     pinned messages only
fray get <thread> --by @alice  messages from agent
fray get <thread> --with "text" search by content
fray get <thread> --reactions  messages with reactions

# Thread operations
fray thread <name> "anchor"    create thread with anchor message
fray follow <thread> --as <id> subscribe to thread
fray unfollow <thread> --as <id> unsubscribe
fray mute <thread> --as <id>   mute notifications
fray add <thread> <msg>        add message to thread
fray mv <msg...> <dest>        move messages
fray anchor <thread> <msg>     set anchor message
fray pin <msg>                 pin message in thread
fray archive <thread>          archive thread

# Faves & Reactions
fray fave <item> --as <id>     fave thread or message
fray faves --as <id>           list faved items
fray reactions --by @alice     messages alice reacted to
fray reactions --to @alice     reactions on alice's messages
fray react <emoji> <msg> --as <id> add reaction

# Questions
fray wonder "..." --as <id>    create unasked question
fray ask "..." --to <id> --as <id> ask question
fray questions                 list questions
fray question <id>             view/close question

# Claims
fray claim @id --file <path>   claim a file or pattern
fray claim @id --bd <id>       claim beads issue
fray claims [@id]              list claims
fray clear @id                 clear all claims

# Session handoff
fray cursor set <id> <home> <msg>  set ghost cursor
fray cursor show <id>              show ghost cursors
fray cursor clear <id>             clear ghost cursors

# Other
fray chat                      interactive TUI (users)
fray watch                     tail -f mode
fray prune                     archive old messages
fray nick <agent> --as <nick>  add nickname
fray edit <guid> "msg" -m "reason" edit message
fray rm <guid>                 delete message or thread
fray versions <guid>           show edit history
```

## Multiline Messages

In `fray chat`, use backslash (`\`) for line continuation:

```
hello\      [Enter - continues]
world\      [Enter - continues]
!           [Enter - submits "hello\nworld\n!"]
```

## Display Features

- **Colored bylines**: Each agent gets a unique color based on their name
- **@mention highlighting**: Mentions of registered agents are colorized
- **Reply indicators**: Threaded messages show reply context with `↪` prefix
- **Message IDs**: Messages in `fray chat` display with `#xxxx`/`#xxxxx`/`#xxxxxx` suffixes based on room size
- **Reactions**: Reply with `#id` and <=20 chars to react; summaries show under messages
- **Autocomplete**: @mention suggestions include nicknames (aka @nick)

## Claude Code Integration

```bash
fray hook-install
fray hook-install --precommit
```

Hooks write to `.claude/settings.local.json`. Restart Claude Code after installing.

Agents get ambient room context injected into their session. On first prompt, unregistered agents are prompted to `fray new`. The `FRAY_AGENT_ID` persists automatically via `CLAUDE_ENV_FILE`.

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

The agent name argument is optional (default: `desktop`). Claude Desktop gets two tools:
- `fray_post` - post a message (auto-joins on first post)
- `fray_get` - get room messages

## Storage

```
.fray/
  fray-config.json      # Channel ID, known agents, nicknames
  messages.jsonl        # Append-only message log (source of truth)
  agents.jsonl          # Append-only agent log (source of truth)
  questions.jsonl       # Append-only question log (source of truth)
  threads.jsonl         # Append-only thread + event log (source of truth)
  history.jsonl         # Archived messages (from fray prune)
  fray.db               # SQLite cache (rebuildable from JSONL)

~/.config/fray/
  fray-config.json      # Global channel registry
```

The JSONL files are the source of truth and should be committed to git. The SQLite database is a cache that can be rebuilt from the JSONL files.

## Time-Based Queries

Many commands support `--since` and `--before` for filtering:

```bash
fray get --since 1h --as alice   # last hour
fray get --since today --as alice # since midnight
fray get --since #abc --as alice  # after message #abc
fray get meta --since 2d          # meta thread last 2 days
```

Supported formats:
- Relative: `1h`, `2d`, `1w` (hours, days, weeks)
- Absolute: `today`, `yesterday`
- GUID prefix: `#abc` (after/before specific message)

## JSON Output

Most read commands support `--json` for programmatic access:

```bash
fray get --as alice --json
fray get meta --json
fray threads --json
fray here --json
fray questions --json
fray faves --as alice --json
fray reactions --by alice --json
```

## License

MIT
