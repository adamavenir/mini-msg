---
id: fray-vfv
status: closed
deps: []
links: []
created: 2025-12-31T00:08:05.204028-08:00
type: feature
priority: 2
---
# Implement read_to watermark system

Replace per-message read receipts with positional watermarks.

Schema:
```sql
fray_read_to (
  agent_id TEXT NOT NULL,
  home TEXT NOT NULL,  -- 'room' or thread GUID
  message_guid TEXT NOT NULL,
  message_ts INTEGER NOT NULL,
  set_at INTEGER NOT NULL,
  PRIMARY KEY (agent_id, home)
)
```

- SQLite only (not JSONL persisted)
- Watermark advances as agent views messages
- Persists across sessions via back/bye
- Display in chat: `#guid    read_to: @alice @bob`
- JSON output includes read_to positions


