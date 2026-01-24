---
id: f-79de
status: closed
deps: []
links: []
created: 2026-01-24T05:00:06Z
type: task
priority: 1
assignee: Adam Avenir
parent: f-4001
tags: [multi-machine, phase1]
---
# Phase 1: Core path helpers and storage version

Add core path helper functions for multi-machine storage.

**Context**: Read docs/MULTI-MACHINE-SYNC-PLAN.md section 1.1 for full details.

**Files to modify**: internal/db/jsonl.go

**Functions to add**:
- GetStorageVersion(projectPath) int - read from fray-config.json
- IsMultiMachineMode(projectPath) bool - check storage_version >= 2
- GetLocalMachineID(projectPath) string - read from local/machine-id
- GetSharedMachinesDirs(projectPath) []string - list all machine directories
- GetLocalMachineDir(projectPath) string - this machine's shared dir
- GetLocalRuntimePath(projectPath) string - local/runtime.jsonl path

**Machine ID file format** (local/machine-id):
{"id": "laptop", "seq": 0, "created_at": 1234567890}

**Tests required**:
- Test GetStorageVersion returns 1 for legacy, 2 for multi-machine
- Test IsMultiMachineMode correctly detects mode
- Test GetLocalMachineID reads from machine-id file
- Test GetSharedMachinesDirs discovers all machine directories

## Acceptance Criteria

- All helper functions implemented
- Unit tests pass
- go test ./... passes
- Changes committed

