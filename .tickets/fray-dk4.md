---
id: fray-dk4
status: closed
deps: [fray-08v]
links: []
created: 2025-12-04T10:03:37.254737-08:00
type: epic
priority: 0
---
# Phase 3: Core Logic

Agent identity and mention parsing logic for bdm.

## Goal
Core business logic utilities for:
- Agent ID parsing, validation, and formatting
- @mention extraction and matching
- Timestamp formatting for human-readable display

## Context
Agent IDs follow a hierarchical format with mandatory version suffix:
- Format: `<role>[.<qualifier>]*.<session_number>`
- Examples: `alice.1`, `alice.frontend.3`, `pm.5.sub.2`

@mentions use prefix matching:
- `@alice` matches alice.1, alice.419, alice.frontend.3
- `@alice.4` matches alice.4, alice.4.sub.1
- `@all` is a special broadcast

See PLAN.md "Decision 4" and "Decision 5" for full rationale.

## Exit Criteria
- Agent ID parsing handles all valid cases
- Mention extraction works with various patterns
- Relative time formatting is human-readable
- Unit tests for all core functions


