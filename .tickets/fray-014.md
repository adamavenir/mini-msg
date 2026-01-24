---
id: fray-014
status: closed
deps: []
links: []
created: 2025-12-31T00:08:05.328679-08:00
type: feature
priority: 2
---
# Add join/leave event messages

Generate event-type messages for agent lifecycle:

- `fray new agent`: Post event 'agent joined'
- `fray back agent` (after bye): Post event 'agent rejoined' 
- `fray back agent` (no prior bye): Post event 'new session: agent' (handoff indicator)
- `fray bye agent`: Post event 'agent left'

These are type='event' messages (not regular agent messages).

Benefits:
- Shows handoff history in chat
- Reduces verbosity vs always showing in real-time
- Provides context for read_to positions


