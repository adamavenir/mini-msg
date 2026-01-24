---
id: fray-3vp
status: closed
deps: [fray-7kr]
links: []
created: 2025-12-19T11:57:09.159982-08:00
type: task
priority: 0
---
# P0.2: Implement JSONL storage layer

Implement JSONL append-only storage layer as source of truth.

Files to create:
- src/db/jsonl.ts - JSONL writers and readers
  - appendMessage(message) → writes to messages.jsonl
  - appendAgent(agent) → writes to agents.jsonl
  - updateProjectConfig(config) → writes to .mm/mm-config.json
  - readMessages() → parses messages.jsonl
  - readAgents() → parses agents.jsonl
  - readProjectConfig() → parses .mm/mm-config.json

Format: One JSON object per line, no trailing commas
Example: {"type":"message","id":"msg-a1b2c3d4",...}

Hook into existing commands:
- mm post → appendMessage()
- mm new → appendAgent() + updateProjectConfig()

References: PLAN.md section 2, beads ~/dev/beads/.beads/issues.jsonl
Critical files: src/db/jsonl.ts (NEW), src/commands/post.ts, src/commands/new.ts


