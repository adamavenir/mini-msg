---
id: fray-cyq
status: open
deps: []
links: []
created: 2025-12-30T16:46:47.56964-08:00
type: feature
priority: 2
---
# Add 'alias' command

Create semantic aliases for messages/notes.

```bash
fray alias #abc123 context       # alias as 'context'
fray alias #def456 handoff       # alias as 'handoff'
```

Usage:
- `#alias` works anywhere (posts, replies, etc.)
- In chat, shows truncated 'quote retweet' with first line + original ID
- `#alias#` embeds the full message content

**Agent namespacing rule**: Agent-created aliases are auto-prefixed with agent name (e.g. `opus.context`) unless `--no-prefix` flag. Prevents collisions while allowing users to create shared aliases like `#context`, `#handoff`.


