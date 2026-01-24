---
id: fray-08v
status: closed
deps: [fray-kit]
links: []
created: 2025-12-04T09:57:56.726228-08:00
type: epic
priority: 0
---
# Phase 2: Database Layer

Schema initialization and typed query functions for bdm.

## Goal
A complete database layer that:
- Discovers and connects to the beads SQLite database
- Initializes bdm-specific tables (agents, messages, linked_projects, config)
- Provides typed query functions for all CRUD operations

## Context
bdm extends the beads SQLite database (`.beads/*.db`) with its own tables.
This follows the beads extension pattern documented in EXTENDING.md.

Tables:
- bdm_agents: Agent presence and identity
- bdm_messages: Room messages with @mentions
- bdm_linked_projects: Cross-project aliases
- bdm_config: Settings (e.g., stale_hours)

See PLAN.md "Schema" section for full SQL definitions.

## Exit Criteria
- Schema creates correctly in beads DB
- All query functions have unit tests
- Types match schema exactly


