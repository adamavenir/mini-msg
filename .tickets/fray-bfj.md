---
id: fray-bfj
status: open
deps: []
links: []
created: 2025-12-19T22:20:27.468386-08:00
type: feature
priority: 3
---
# Poller plugin system for external event sources

Add a plugin system for polling external systems and injecting messages into the mm room. Like webhooks but pull-based since there's no endpoint.

## Design

**Configuration** (in mm-config.json):
```json
{
  "pollers": [
    {
      "package": "mm-poller-beads",
      "config": {
        "path": ".beads"
      }
    },
    {
      "package": "mm-poller-github",
      "config": {
        "repo": "adamavenir/mini-msg",
        "events": ["issues", "pull_requests"],
        "token_env": "GITHUB_TOKEN"
      },
      "interval": 60000
    }
  ]
}
```

**Polling triggers:**
- Lazy: Any `mm` command runs all pollers once before executing
- Active: `mm watch` / `mm chat` run pollers on interval in addition to lazy

**Cursor storage** (.mm/poller-state.json):
```json
{
  "mm-poller-beads": { "cursor": "2025-12-19T10:30:00Z", "lastPoll": 1734567890 },
  "mm-poller-github": { "cursor": "etag:abc123", "lastPoll": 1734567850 }
}
```

**Poller interface:**
```typescript
export interface Poller {
  name: string;
  defaultInterval?: number;
  poll(ctx: PollerContext): Promise<PollResult>;
}

export interface PollerContext {
  config: Record<string, unknown>;
  cursor: string | null;
  projectPath: string;
}

export interface PollResult {
  messages: PollerMessage[];
  cursor: string | null;
}

export interface PollerMessage {
  body: string;
  mentions?: string[];
  source?: string;        // e.g. "github:issues/123"
}
```

mm creates messages with `from_agent: poller.name`, `type: 'event'`.

## Implementation tasks
- [ ] Define poller interface in src/pollers/types.ts
- [ ] Add poller config schema to mm-config.json
- [ ] Add poller-state.json read/write helpers
- [ ] Create runPollers() function that loads and executes configured pollers
- [ ] Call runPollers() lazily from shared.ts (before command execution)
- [ ] Call runPollers() on interval in watch.ts and chat.ts
- [ ] Create mm-poller-beads package (watches .beads/issues.jsonl)
- [ ] Create mm-poller-github package (optional, example)


