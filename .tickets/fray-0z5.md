---
id: fray-0z5
status: closed
deps: []
links: []
created: 2025-12-04T22:53:00.040842-08:00
type: task
priority: 2
---
# Investigate integration strategy

We need to develop a strategy for beads-messenger that makes it trivial for users to onboard agents. The goal should be 3 steps:

- run npm install
- config/setup (even better if also a command)
- onboard agent (should be a command - 'new' could onboard them AND update them in one command)

Look at claude code hooks, claude code skills, beads' `bd quickstart` example for onboarding.

We also need to create an MCP interface for Claude Desktop App, which can't necessarily use a cli but can use an MCP server that makes calls locally.


