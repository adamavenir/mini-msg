---
id: fray-oa5
status: closed
deps: [fray-6cj, fray-1wj]
links: []
created: 2025-12-04T10:00:19.532948-08:00
type: task
priority: 1
parent: fray-08v
---
# Implement config and linked project query functions

Create query functions for config and linked projects in src/db/queries.ts.

## Functions to Implement

```typescript
// src/db/queries.ts (config and project functions)

import type { LinkedProject, ConfigEntry } from '../types.js';
import type Database from 'better-sqlite3';

/**
 * Get config value by key.
 * @returns value or undefined if not set
 */
export function getConfig(db: Database, key: string): string | undefined;

/**
 * Set config value.
 * Uses INSERT OR REPLACE for upsert.
 */
export function setConfig(db: Database, key: string, value: string): void;

/**
 * Get all config entries.
 */
export function getAllConfig(db: Database): ConfigEntry[];

/**
 * Get linked project by alias.
 */
export function getLinkedProject(db: Database, alias: string): LinkedProject | undefined;

/**
 * Get all linked projects.
 */
export function getLinkedProjects(db: Database): LinkedProject[];

/**
 * Link a project (store in database).
 * @param alias - short name for --project flag
 * @param path - absolute path to project root
 * Note: Caller is responsible for validating path exists and contains .beads/
 */
export function linkProject(db: Database, alias: string, path: string): void;

/**
 * Unlink a project.
 * @returns true if project was removed, false if didn't exist
 */
export function unlinkProject(db: Database, alias: string): boolean;
```

## Implementation Notes
- Config uses INSERT OR REPLACE for upsert semantics
- linkProject is a pure database function - path validation happens in the CLI command layer (bdm-1ur)
- Store absolute paths (caller should resolve relative paths before calling)

## Default Config Values
- stale_hours: "4" (inserted by schema initialization)

## Files
- src/db/queries.ts (add config and project functions)

## Acceptance Criteria
- Config get/set works correctly
- Linked projects store paths as provided
- Unit tests for all functions


