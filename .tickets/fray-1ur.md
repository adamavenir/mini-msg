---
id: fray-1ur
status: closed
deps: [fray-2vy, fray-oa5]
links: []
created: 2025-12-04T10:10:07.749068-08:00
type: task
priority: 1
parent: fray-yh4
---
# Implement cross-project commands (link, unlink, projects)

Implement commands for managing linked projects.

## Commands

### bdm link <alias> <path>
Link another beads project for cross-project messaging.

```bash
bdm link api ~/dev/api-service
bdm link frontend ../frontend
```

**Implementation:**
1. Resolve path to absolute
2. Verify path contains .beads/ directory
3. Store in bdm_linked_projects table
4. Output: "Linked 'api' → /Users/me/dev/api-service"

**Validation:**
- Path must exist
- Path must contain .beads/ (be a beads project)
- Alias must be valid identifier (alphanumeric, hyphens, underscores)
- Error if alias already exists (suggest --force to overwrite)

### bdm unlink <alias>
Remove a linked project.

```bash
bdm unlink api
```

**Implementation:**
1. Delete from bdm_linked_projects
2. Output: "Unlinked 'api'"

**Error:**
- Alias doesn't exist: "No project linked as 'api'"

### bdm projects
List current and linked projects.

```bash
bdm projects
```

**Output:**
```
CURRENT:
  /Users/me/dev/beads-messenger

LINKED:
  api       /Users/me/dev/api-service
  frontend  /Users/me/dev/frontend
```

## Using --project Flag
After linking, any command can use --project:

```bash
bdm --project api here
bdm --project api post --as frontend.1 "@backend.2 heads up"
```

**Implementation in cli.ts:**
1. Check for --project option
2. Look up alias in bdm_linked_projects
3. Open that project's database instead
4. All subsequent commands operate there

## Files
- src/commands/link.ts
- src/commands/unlink.ts
- src/commands/projects.ts
- src/cli.ts (--project handling)

## Acceptance Criteria
- Link stores absolute path
- Link validates target is beads project
- Unlink removes entry
- Projects lists all linked projects
- --project flag switches context correctly


