---
id: fray-8xb
status: closed
deps: []
links: []
created: 2025-12-04T10:17:20.919183-08:00
type: task
priority: 1
parent: fray-po1
---
# Implement comprehensive error handling

Create error handling utilities for clear, actionable error messages.

## Error Categories

### Project Errors
- Not in beads project: "Not in a beads project. Run 'bd init' first."
- Linked project not found: "Linked project 'api' not found at /path/to/project"
- Linked path not a beads project: "Path /path/to/dir is not a beads project"

### Agent Errors
- Unknown agent: "Unknown agent: alice.999"
- Agent not active: "Agent alice.419 is not active. Use 'bdm hi alice.419' to resume."
- Agent already active: "alice.419 is already active"
- Agent already left: "alice.419 has already left"
- Invalid agent ID: "Invalid agent ID: 'alice' (missing version number)"

### Message Errors
- Empty message: "Message cannot be empty"

### Config Errors
- Unknown key: "Unknown config key: foo"
- Invalid value: "Invalid value for stale_hours: must be a positive integer"

## Implementation

```typescript
// src/errors.ts

export class BdmError extends Error {
  constructor(message: string, public readonly code: string) {
    super(message);
    this.name = 'BdmError';
  }
}

export class NotInProjectError extends BdmError {
  constructor() {
    super("Not in a beads project. Run 'bd init' first.", 'NOT_IN_PROJECT');
  }
}

export class UnknownAgentError extends BdmError {
  constructor(agentId: string) {
    super(\`Unknown agent: \${agentId}\`, 'UNKNOWN_AGENT');
  }
}

// etc.
```

## CLI Error Wrapper

```typescript
// src/cli.ts

async function run(fn: () => Promise<void>) {
  try {
    await fn();
    process.exit(0);
  } catch (err) {
    if (err instanceof BdmError) {
      console.error(\`Error: \${err.message}\`);
      process.exit(1);
    }
    // Unexpected error
    console.error('Unexpected error:', err);
    process.exit(2);
  }
}
```

## Exit Codes
- 0: Success
- 1: Known error (BdmError)
- 2: Unexpected error

## Files
- src/errors.ts
- src/cli.ts (update error handling)

## Acceptance Criteria
- All error messages are clear and actionable
- Exit codes are consistent
- Stack traces not shown for expected errors
- Unexpected errors show debug info


