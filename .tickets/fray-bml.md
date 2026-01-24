---
id: fray-bml
status: closed
deps: [fray-3po, fray-fr5]
links: []
created: 2025-12-04T09:56:48.142467-08:00
type: task
priority: 1
parent: fray-kit
---
# Create directory structure and entry points

Create the project directory structure and minimal entry point files.

## Directory Structure
```
beads-messenger/
├── src/
│   ├── cli.ts           # Commander program setup
│   ├── commands/        # Command implementations (empty for now)
│   ├── db/              # Database layer (empty for now)
│   ├── core/            # Business logic (empty for now)
│   └── types.ts         # Type definitions (placeholder)
├── bin/
│   └── bdm.ts           # CLI entry point (hashbang)
├── tests/               # Test files (empty for now)
├── package.json
├── tsconfig.json
└── tsup.config.ts
```

## Files to Create

### bin/bdm.ts
```typescript
#!/usr/bin/env node
import { main } from '../src/cli.js';
main();
```

### src/cli.ts
Note: Importing version from package.json is tricky with ESM. Use one of:
- Read package.json with fs and JSON.parse
- Hardcode version and update on release
- Use a build-time replacement

Simple approach for now:
```typescript
import { Command } from 'commander';

const VERSION = '0.1.0';  // TODO: sync with package.json

export function main() {
  const program = new Command();
  program
    .name('bdm')
    .description('Agent messaging for beads projects')
    .version(VERSION);
  
  program.parse();
}
```

### src/types.ts
```typescript
// Placeholder - full types defined in bdm-6cj
export interface Agent {
  agent_id: string;
}

export interface Message {
  id: number;
}
```

### Empty directories
Create with .gitkeep or placeholder files:
- src/commands/
- src/db/
- src/core/
- tests/

## Acceptance Criteria
- `npm run build` completes successfully
- `./dist/bin/bdm.js --version` outputs "0.1.0"
- `./dist/bin/bdm.js --help` shows usage
- All directories exist


