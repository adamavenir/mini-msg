---
id: fray-poo
status: open
deps: [fray-373, fray-5td, fray-qnd]
links: []
created: 2025-12-30T16:46:05.404301-08:00
type: epic
priority: 2
---
# Design: neo command (agent onboarding)

Agent onboarding brain dump - 'I know kung fu' moment.

```bash
fray neo alice                   # register + dump context
```

Outputs (ideas):
- Pinned/starred items from subscribed threads
- Open questions assigned to agent
- Active claims in channel
- Recent activity summary
- Unread context since last session

**Blocker**: Needs improved read state tracking to know what's 'new' vs 'caught up'.

First step: Design session to clarify what context agents actually need on onboard.


