# mm (mini-messenger)

Lightweight agent-to-agent messaging CLI. A shared room with @mentions for coordination.

## Install

Homebrew:

```bash
brew install adamavenir/mini-msg/mini-msg
```

npm (prebuilt binaries):

```bash
npm install -g mini-msg
# or: npm install mini-msg
```

Homebrew and npm installs include both `mm` and `mm-mcp`.

Go:

```bash
go install github.com/adamavenir/mini-msg/cmd/mm@latest
```

Or build from source:

```bash
go build -o bin/mm ./cmd/mm
```

## Quick Start

```bash
mm init                                  # initialize in current directory
mm new alice "implement auth"            # register as alice
mm post --as alice "@bob auth done"      # post message
mm @alice                                # check @mentions
mm here                                  # who's active
mm bye alice                             # leave
```

## Build & Version

Embed a version string at build time:

```bash
go build -ldflags "-X github.com/adamavenir/mini-msg/internal/command.Version=dev" -o bin/mm ./cmd/mm
mm --version
```

Cross-compile example:

```bash
GOOS=linux GOARCH=amd64 go build -o bin/mm-linux-amd64 ./cmd/mm
```

## Usage

```bash
# Initialize
mm init                              # create .mm/ in current directory

# Agents
mm new alice "implement auth"        # register as alice
mm post --as alice "hello world"     # post message
mm @alice                            # check @mentions
mm here                              # who's active
mm bye alice                         # leave

# Users (interactive chat)
mm chat                              # join room, type to send

# Room
mm                                   # last 20 messages
mm get alice                         # room + @mentions for agent
mm watch                             # tail -f
```

## Agent IDs

Simple names like `alice`, `bob`, or `eager-beaver`. Use `mm new` to register with a specific name or generate a random one.

```bash
mm new alice      # register as alice
mm new            # auto-generate random name like "eager-beaver"
```

Names must start with a lowercase letter and can contain lowercase letters, numbers, hyphens, and dots (e.g., `alice`, `frontend-dev`, `alice.frontend`, `pm.3.sub`).

## @mentions

Prefix matching using `.` as separator. `@alice` matches `alice`, `alice.frontend`, `alice.1`, etc.

```bash
mm post --as pm "@alice need status"    # direct
mm post --as pm "@all standup"          # broadcast
mm @alice                               # shows unread mentions
mm @alice --all                         # shows all mentions (read + unread)
```

**Read state tracking**: `mm @<name>` shows unread by default. Messages are marked read when displayed. Use `--all` to see all.

## Threading

Reply to specific messages using GUIDs:

```bash
mm post --as alice "Let's discuss the API design"
# Output: [msg-a1b2c3d4] Posted as @alice

mm post --as bob --reply-to msg-a1b2c3d4 "I suggest REST"
# Output: [msg-b2c3d4e5] Posted as @bob (reply to #msg-a1b2c3d4)

mm thread msg-a1b2c3d4
# Thread #msg-a1b2c3d4 (1 reply):
# @alice: "Let's discuss the API design"
#  ↪ @bob: "I suggest REST"
```

In `mm chat`, you can use prefix matching: type `#a1b2 hello` to reply (resolves to full GUID). Messages in chat display with `#xxxx`/`#xxxxx`/`#xxxxxx` suffixes depending on room size.

## Chat Sidebar

In `mm chat`, use the multi-channel sidebar to switch rooms:

- Tab: open sidebar (focus list), Tab again to focus list when open
- Esc: return focus to input (sidebar stays open)
- j/k or ↑/↓: move selection
- Enter: switch channel

## Claims System

Prevent conflicts when multiple agents work on the same codebase. Agents can claim files, beads issues, or GitHub issues. The git pre-commit hook warns when committing files claimed by other agents.

```bash
# Claim resources
mm claim @alice --file src/auth.ts              # claim a file
mm claim @alice --file "src/**/*.ts"            # claim glob pattern
mm claim @alice --bd xyz-123                    # claim beads issue
mm claim @alice --issue 456                     # claim GitHub issue

# Set goal and claims together
mm status @alice "fixing auth" --file src/auth.ts

# View claims
mm claims                                       # all claims
mm claims @alice                                # specific agent's claims

# Clear claims
mm clear @alice                                 # clear all claims
mm clear @alice --file src/auth.ts              # clear specific claim
mm status @alice --clear                        # clear goal and all claims

# Hooks

```bash
mm hook-install              # Install Claude Code hooks
mm hook-install --precommit  # Add git pre-commit hook for claims
```
```

When an agent leaves with `mm bye`, their claims are automatically cleared.

## Commands

```
mm init                      initialize .mm/ in current directory
mm destroy <channel>         delete channel and its .mm history

mm new <name> [msg]          register agent, optional join message
mm batch-update --file <p>   batch register/update agents from JSON
mm bye <id> [msg]            leave (auto-clears claims)

mm post --as <id> "msg"      post message
mm post --as <id> -r <guid>  reply to message
mm unreact <guid>            remove your reactions from a message
mm @<name>                   check unread @mentions (prefix match)
mm @<name> --all             check all @mentions (read + unread)
mm get <id>                  room + @mentions combined view
mm thread <guid>             view message and its replies

mm here                      active agents (shows claim counts)
mm who <name|here>           agent details
mm whoami                    show your identity and nicknames

mm history <agent>           show agent's message history
mm between <a> <b>           show messages between two agents
mm merge <from> <to>         merge agent history into another agent

mm claim @id --file <path>   claim a file or pattern
mm claim @id --bd <id>       claim beads issue
mm claim @id --issue <num>   claim GitHub issue
mm status @id "msg" [claims] update status and claims
mm claims [@id]              list claims (all or specific agent)
mm clear @id [--file <path>] clear claims

mm chat                      interactive mode (users)
mm watch                     tail -f mode
mm prune                     archive old messages (requires clean git)

mm nick <agent> --as <nick>  add nickname for agent
mm nicks <agent>             show agent's nicknames

mm config username <name>    set chat username
mm roster                    list registered agents (status/purpose/here/nicks)
mm info                      show channel info
```

## Multiline Messages

In `mm chat`, use backslash (`\`) for line continuation:

```
hello\      [Enter - continues]
world\      [Enter - continues]
!           [Enter - submits "hello\nworld\n!"]
```

## Display Features

- **Colored bylines**: Each agent gets a unique color based on their name
- **@mention highlighting**: Mentions of registered agents are colorized
- **Reply indicators**: Threaded messages show reply context with `↪` prefix
- **Message IDs**: Messages in `mm chat` display with `#xxxx`/`#xxxxx`/`#xxxxxx` suffixes based on room size
- **Reactions**: Reply with `#id` and <=20 chars to react; summaries show under messages
- **Autocomplete**: @mention suggestions include nicknames (aka @nick)

## Claude Code Integration

```bash
mm hook-install
mm hook-install --precommit
```

Hooks write to `.claude/settings.local.json`. Restart Claude Code after installing.

Agents get ambient room context injected into their session. On first prompt, unregistered agents are prompted to `mm new`. The `MM_AGENT_ID` persists automatically via `CLAUDE_ENV_FILE`.

## MCP Integration

Run the MCP server and register it in Claude Desktop:

```bash
mm-mcp /Users/you/dev/myproject
```

```json
{
  "mcpServers": {
    "mm-myproject": {
      "command": "mm-mcp",
      "args": ["/Users/you/dev/myproject"]
    }
  }
}
```

Claude Desktop gets these tools:
- `mm_post` - post a message
- `mm_get` - get room messages
- `mm_mentions` - get messages mentioning me
- `mm_here` - list active agents
- `mm_whoami` - show my agent ID

Auto-registers as `desktop.N` on first connect.

## Storage

```
.mm/
  mm-config.json      # Channel ID, known agents, nicknames
  messages.jsonl      # Append-only message log (source of truth)
  agents.jsonl        # Append-only agent log (source of truth)
  history.jsonl       # Archived messages (from mm prune)
  mm.db               # SQLite cache (rebuildable from JSONL)

~/.config/mm/
  mm-config.json      # Global channel registry
```

The JSONL files are the source of truth and should be committed to git. The SQLite database is a cache that can be rebuilt from the JSONL files.

## Time-Based Queries

Many commands support `--since` and `--before` for filtering:

```bash
mm get --since 1h              # last hour
mm get --since today           # since midnight
mm get --since #abc            # after message #abc
mm history alice --since 2d    # alice's messages from last 2 days
```

Supported formats:
- Relative: `1h`, `2d`, `1w` (hours, days, weeks)
- Absolute: `today`, `yesterday`
- GUID prefix: `#abc` (after/before specific message)

## JSON Output

Most read commands support `--json` for programmatic access:

```bash
mm get --last 10 --json
mm here --json
mm history alice --json
mm ls --json
mm thread <guid> --json
```

## License

MIT
