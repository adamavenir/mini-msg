---
id: fray-8xf
status: closed
deps: []
links: []
created: 2025-12-05T16:35:17.103598-08:00
type: task
priority: 0
---
# Design: Claude Desktop project discovery and MCP architecture

Design how Claude Desktop discovers and connects to beads projects via MCP.

## Context

**bdm** (beads-messenger) is a CLI for agent-to-agent messaging in beads projects. It stores messages in `.beads/*.db` (same SQLite database as beads issue tracker). Current features:
- Agents register with `bdm new <name>` and get IDs like `alice.1`
- Messages posted with `bdm post --as alice.1 "message"`
- @mentions for routing (`@bob.2`, `@all`)
- `bdm chat` for interactive user participation

**Goal**: Add an MCP server so Claude Desktop can join rooms as an agent.

**MCP** (Model Context Protocol) lets Claude Desktop connect to external tools/data:
- **Tools**: Actions Claude calls on-demand (`bdm_post`, `bdm_get`)
- **Resources**: Data auto-loaded into context (`bdm://room` with recent messages)

## Questions to Answer

1. **Project Discovery**
   - How does Claude Desktop know which projects exist?
   - Options: config file, auto-discovery, manual add via UI
   - Where would config live? `~/.config/bdm/projects.json`?

2. **Connection Model**
   - One MCP server per project? Or one server serving multiple projects?
   - How to switch between projects in conversation?
   - Can Claude Desktop handle multiple MCP connections?

3. **Agent Identity**
   - Auto-register on connect? Or require explicit `bdm_new`?
   - Persist agent ID across sessions? (Same `claude.1` every time vs new ID)
   - Storage: config file, server memory, or stateless?

4. **Tools vs Resources**
   - **Tools** (likely): `bdm_new`, `bdm_post`, `bdm_get`, `bdm_here`, `bdm_bye`
   - **Resources** (maybe): `bdm://room`, `bdm://mentions/{agent}`
   - Start tools-only for simplicity? Resources add ambient awareness.

5. **Configuration**
   - Claude Desktop config: `~/Library/Application Support/Claude/claude_desktop_config.json`
   - How to add bdm-mcp there?
   - Example: `{"mcpServers": {"bdm": {"command": "npx", "args": ["bdm-mcp", "/path/to/project"]}}}`

## Deliverable

Write to PLAN-mcp.md:
- Recommended architecture diagram
- User flow for connecting Claude Desktop to a project
- MCP server interface (tools + resources definitions)
- Config file formats
- Implementation phases

## Dependencies

- Findings from bdm-0od (beads daemon investigation)

## Parent Epic

bdm-ae4: MCP Server for Claude Desktop Integration


