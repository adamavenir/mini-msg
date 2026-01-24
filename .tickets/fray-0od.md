---
id: fray-0od
status: closed
deps: []
links: []
created: 2025-12-05T16:35:05.067265-08:00
type: task
priority: 0
---
# Investigate beads daemon architecture

Investigate how beads implements its per-project daemon and determine how bdm should integrate.

## Context

**bdm** (beads-messenger) is a CLI for agent-to-agent messaging in beads projects. We want to add an MCP server so Claude Desktop can participate in chat rooms. Before implementing, we need to understand how beads handles daemons.

**MCP** (Model Context Protocol) is Anthropic's protocol for connecting Claude to external tools/data. MCP servers expose:
- **Tools**: Actions Claude can call (like `bdm_post`)
- **Resources**: Data loaded into Claude's context (like room messages)

**Why investigate beads daemon?** beads reportedly uses one daemon per project. We need to understand this before deciding whether bdm's MCP server should:
- Extend the beads daemon
- Run its own daemon
- Be stateless (no daemon)

## Questions to Answer

1. **How does the beads daemon work?**
   - What triggers daemon startup?
   - How does it discover/bind to a project?
   - What does it serve? (HTTP? Unix socket? IPC?)
   - How long does it live? Auto-shutdown on idle?

2. **Where is the daemon code?**
   - Find the daemon implementation in beads source
   - Understand the architecture

3. **Extension points**
   - Can beads daemon be extended with plugins?
   - Should bdm add its own endpoints to beads daemon?
   - Or run a separate daemon that coordinates with beads?

4. **MCP server patterns**
   - How do other MCP servers handle per-project state?
   - Does MCP SDK support daemon mode or is it stdio-per-call?
   - Look at: https://modelcontextprotocol.io/docs

## How to Investigate

```bash
# beads source is likely at ~/dev/beads or similar
# Search for daemon-related code:
grep -r "daemon" --include="*.go" ~/dev/beads
grep -r "server" --include="*.go" ~/dev/beads
grep -r "listen" --include="*.go" ~/dev/beads

# Check MCP SDK docs
# https://modelcontextprotocol.io/docs
```

## Deliverable

Write findings to PLAN-mcp.md or comment on this issue with:
- Summary of beads daemon architecture
- Recommended approach for bdm MCP server
- Risks and tradeoffs

## Parent Epic

bdm-ae4: MCP Server for Claude Desktop Integration


