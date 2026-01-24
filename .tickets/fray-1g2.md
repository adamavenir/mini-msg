---
id: fray-1g2
status: closed
deps: []
links: []
created: 2025-12-31T08:01:10.00535-08:00
type: feature
priority: 2
---
# Add PreCompact hook with /land reminder

Add PreCompact hook that fires when Claude context is compacting.

Inject reminder:
```
[fray] Context compacting. Preserve your work:
1. fray note "# Handoff ..." --as <you>
2. bd close <completed-issues>  
3. fray bye <you>

Or run /land for full checklist.
```

This is automatic insurance against context loss.

Files: internal/command/hook_precompact.go (new), internal/command/hook_install.go


