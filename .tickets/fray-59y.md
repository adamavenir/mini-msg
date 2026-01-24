---
id: fray-59y
status: open
deps: [fray-ep0]
links: []
created: 2025-12-09T06:12:15.797525-08:00
type: epic
priority: 3
---
# Revisit event notification design

After silencing system messages by default, revisit how to surface useful event notifications.

## Context

We're silencing all system messages (issue status changes, etc.) by default because they flood the chat. But some events might be useful to agents.

## Future Considerations

1. **Context-aware events**: Only show events for issues mentioned in recent conversation
2. **Event summaries**: Instead of one message per event, batch related events (e.g., "3 issues closed")
3. **Agent-specific subscriptions**: Let agents subscribe to specific issues or patterns
4. **Event digest**: Daily/hourly summary instead of real-time flood

## Blocked By

- bdm-ep0 (silence by default) must be implemented first

## Questions

- Do agents actually need to see issue events in chat?
- Should events be in a separate channel/view?
- What's the right granularity for event notifications?


