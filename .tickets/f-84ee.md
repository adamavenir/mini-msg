---
id: f-84ee
status: closed
deps: [f-2312]
links: []
created: 2026-01-24T05:07:58Z
type: task
priority: 2
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase5]
---
# Phase 5: @agent@machine display logic

Implement display logic that shows @agent@machine when disambiguation is needed.

**Context**: Read docs/MULTI-MACHINE-SYNC-SPEC.md Display Rules section.

**Files to modify**: internal/chat/ (display formatting code)

**Display rule**:
- Show @agent@machine only when agent has posted from multiple origins
- Based on historical message data, NOT live presence

```go
func formatAgentDisplay(msg Message, ctx ConversationContext) string {
    if ctx.AgentHasMultipleOrigins(msg.FromAgent) {
        return fmt.Sprintf("@%s@%s", msg.FromAgent, msg.Origin)
    }
    return "@" + msg.FromAgent
}

func (ctx *ConversationContext) AgentHasMultipleOrigins(agentID string) bool {
    origins := ctx.db.GetDistinctOriginsForAgent(agentID)
    return len(origins) > 1
}
```

**Database query**: Add GetDistinctOriginsForAgent(agentID) query.

**Tests required**:
- Test single-origin agent shows @opus
- Test multi-origin agent shows @opus@laptop
- Test display is consistent across machines

## Acceptance Criteria

- Display logic implemented
- Query for distinct origins
- Correct format based on origin count
- Unit tests pass
- go test ./... passes
- Changes committed

