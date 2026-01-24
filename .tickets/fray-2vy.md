---
id: fray-2vy
status: closed
deps: [fray-08v]
links: []
created: 2025-12-04T10:08:03.90436-08:00
type: task
priority: 1
parent: fray-yh4
---
# Set up CLI framework with Commander.js

Enhance src/cli.ts to set up the full Commander.js program with global options.

## Requirements

1. Global options that apply to all commands:
   - `--project <alias>`: Operate in linked project context
   - `--json`: Output in JSON format (for scripting)

2. Command structure:
   - Subcommands for each action (new, hi, bye, here, who, post, link, etc.)
   - Default command (no subcommand): show room history

3. Database initialization:
   - Discover beads project
   - Open database and init schema
   - Handle --project to switch context

## Implementation

```typescript
// src/cli.ts

import { Command } from 'commander';

export function main() {
  const program = new Command();
  
  program
    .name('bdm')
    .description('Agent messaging for beads projects')
    .version('0.1.0')
    .option('--project <alias>', 'operate in linked project')
    .option('--json', 'output in JSON format');

  // Add subcommands
  program.addCommand(newCommand());
  program.addCommand(hiCommand());
  program.addCommand(byeCommand());
  program.addCommand(hereCommand());
  program.addCommand(whoCommand());
  program.addCommand(postCommand());
  program.addCommand(linkCommand());
  program.addCommand(unlinkCommand());
  program.addCommand(projectsCommand());
  program.addCommand(configCommand());

  // Default action (show messages)
  program
    .option('--last <n>', 'show last N messages', '20')
    .option('--since <id>', 'show messages after ID')
    .option('--all', 'show all messages')
    .option('--watch', 'tail -f style')
    .action(messagesAction);

  // @mentions shorthand: bdm @alice
  // Handled by checking if first arg starts with @
  
  program.parse();
}
```

## Command Pattern
Each command file exports a function that returns a Command:

```typescript
// src/commands/new.ts
import { Command } from 'commander';

export function newCommand(): Command {
  return new Command('new')
    .description('Create new agent session')
    .argument('<name>', 'base name for agent')
    .option('--goal <goal>', 'agent goal')
    .option('--bio <bio>', 'agent bio')
    .action(async (name, options, cmd) => {
      // Implementation
    });
}
```

## Files
- src/cli.ts (update)
- src/commands/index.ts (export all commands)

## Acceptance Criteria
- `bdm --help` shows all commands and options
- `bdm <command> --help` shows command-specific help
- `--project` flag works with all commands
- Error handling wrapper catches and formats errors


