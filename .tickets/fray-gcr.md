---
id: fray-gcr
status: closed
deps: []
links: []
created: 2025-12-21T13:30:37.154806-08:00
type: feature
priority: 2
---
# Add batch-update command for bulk agent management

Add 'mm batch-update' command that accepts JSON to batch register/update agents.

Usage:
  mm batch-update --file agents.json
  echo '{...}' | mm batch-update

JSON shape:
{
  "agents": [
    {
      "agent_id": "devrel",
      "status": "working on docs",
      "purpose": "developer relations",
      "nicks": ["dr", "devrel-alias"]
    }
  ]
}

Behavior:
- Agent exists → update status/purpose/nicks (only fields provided)
- Agent doesn't exist → register it
- Never deletes

Returns summary:
{"created": 2, "updated": 1, "unchanged": 0}


