---
id: fray-ae4
status: closed
deps: []
links: []
created: 2025-12-05T16:34:48.910772-08:00
type: epic
priority: 2
---
# MCP Server for Claude Desktop Integration

Add an MCP server to bdm that allows Claude Desktop to participate in project chat rooms as an agent.

## Context

**bdm** (beads-messenger) is a CLI for agent-to-agent messaging in beads projects. Currently works via:
- CLI commands (`bdm post`, `bdm get`, `bdm new`)
- Claude Code hooks (ambient context injection)
- Interactive chat (`bdm chat`)

**MCP** (Model Context Protocol) is Anthropic's protocol for connecting Claude to external tools/data. Adding an MCP server would let **Claude Desktop** (not just Claude Code) participate in project rooms.

## Goal

Claude Desktop users can:
- Connect to a beads project
- Register as an agent (`bdm_new`)
- Post messages (`bdm_post`) 
- Read room + @mentions (`bdm_get`)
- See who's active (`bdm_here`)
- Leave (`bdm_bye`)

## Key Questions

1. **Daemon architecture**: beads uses one daemon per project. Should bdm:
   - Extend the beads daemon?
   - Run its own daemon?
   - Be stateless (discover project per-call)?

2. **Multi-project**: How does Claude Desktop know which projects to connect to?

3. **Resources vs Tools**: 
   - Tools: on-demand actions
   - Resources: auto-loaded context (room messages in context window)

4. **Agent identity**: How to persist agent ID across MCP calls?

## Planned Implementation

```
src/mcp/
  server.ts      # MCP server entry
  tools.ts       # Tool handlers  
  resources.ts   # Resource handlers (if needed)
bin/
  bdm-mcp.ts     # Entry point for "npx bdm-mcp"
```

## Child Issues

| Issue | Title | Status |
|-------|-------|--------|
| bdm-0od | Investigate beads daemon architecture | open |
| bdm-8xf | Design: Claude Desktop project discovery | open |
| TBD | Implement MCP server with tools | - |
| TBD | Add resources for ambient context | - |

## Resources

- MCP docs: https://modelcontextprotocol.io/docs
- MCP TypeScript SDK: @modelcontextprotocol/sdk
- Claude Desktop config: ~/Library/Application Support/Claude/claude_desktop_config.json


