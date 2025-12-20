import { Command } from 'commander';
import { existsSync, mkdirSync, readFileSync, writeFileSync, chmodSync } from 'fs';
import { join } from 'path';
import { execSync } from 'child_process';

export function hookInstallCommand(): Command {
  return new Command('hook-install')
    .description('Install Claude Code hooks for mm integration')
    .option('--dry-run', 'show what would be written without writing')
    .option('--precommit', 'also install git pre-commit hook for claim conflict detection')
    .action((options, cmd) => {
      const projectDir = process.env.CLAUDE_PROJECT_DIR || process.cwd();
      const claudeDir = join(projectDir, '.claude');
      const settingsPath = join(claudeDir, 'settings.local.json');

      const hooksConfig = {
        hooks: {
          SessionStart: [
            {
              matcher: 'startup',
              hooks: [
                {
                  type: 'command',
                  command: 'mm hook-session startup',
                  timeout: 10,
                },
              ],
            },
            {
              matcher: 'resume',
              hooks: [
                {
                  type: 'command',
                  command: 'mm hook-session resume',
                  timeout: 10,
                },
              ],
            },
          ],
          UserPromptSubmit: [
            {
              hooks: [
                {
                  type: 'command',
                  command: 'mm hook-prompt',
                  timeout: 5,
                },
              ],
            },
          ],
        },
      };

      if (options.dryRun) {
        console.log('Would write to:', settingsPath);
        console.log(JSON.stringify(hooksConfig, null, 2));
        return;
      }

      // Ensure .claude directory exists
      if (!existsSync(claudeDir)) {
        mkdirSync(claudeDir, { recursive: true });
      }

      // Merge with existing settings if present
      let existingSettings: Record<string, unknown> = {};
      if (existsSync(settingsPath)) {
        try {
          existingSettings = JSON.parse(readFileSync(settingsPath, 'utf-8'));
        } catch {
          // Ignore parse errors, start fresh
        }
      }

      // Merge hooks config
      const merged = {
        ...existingSettings,
        hooks: {
          ...(existingSettings.hooks as Record<string, unknown> || {}),
          ...hooksConfig.hooks,
        },
      };

      writeFileSync(settingsPath, JSON.stringify(merged, null, 2) + '\n');
      console.log(`Hooks installed to ${settingsPath}`);
      console.log('');
      console.log('Installed hooks:');
      console.log('  SessionStart (startup/resume) - prompts agent registration or injects context');
      console.log('  UserPromptSubmit - injects room messages and @mentions before each prompt');

      // Install git pre-commit hook if requested
      if (options.precommit) {
        installPrecommitHook(projectDir, options.dryRun);
      }

      console.log('');
      console.log('Restart Claude Code to activate hooks.');
    });
}

function installPrecommitHook(projectDir: string, dryRun: boolean): void {
  // Find git root
  let gitRoot: string;
  try {
    gitRoot = execSync('git rev-parse --show-toplevel', {
      encoding: 'utf-8',
      cwd: projectDir,
      stdio: ['pipe', 'pipe', 'pipe'],
    }).trim();
  } catch {
    console.log('');
    console.log('⚠️  Not in a git repository, skipping pre-commit hook installation');
    return;
  }

  const hooksDir = join(gitRoot, '.git', 'hooks');
  const precommitPath = join(hooksDir, 'pre-commit');

  const hookScript = `#!/bin/sh
# mm pre-commit hook - detects file claim conflicts
# Installed by: mm hook-install --precommit

mm hook-precommit
`;

  if (dryRun) {
    console.log('');
    console.log('Would write git pre-commit hook to:', precommitPath);
    console.log(hookScript);
    return;
  }

  // Check if pre-commit hook already exists
  if (existsSync(precommitPath)) {
    const existing = readFileSync(precommitPath, 'utf-8');
    if (existing.includes('mm hook-precommit')) {
      console.log('');
      console.log('Git pre-commit hook already installed');
      return;
    }

    // Append to existing hook
    const updated = existing.trimEnd() + '\n\n# mm file claim conflict detection\nmm hook-precommit\n';
    writeFileSync(precommitPath, updated);
    console.log('');
    console.log(`Added mm hook to existing pre-commit hook at ${precommitPath}`);
    return;
  }

  // Ensure hooks directory exists
  if (!existsSync(hooksDir)) {
    mkdirSync(hooksDir, { recursive: true });
  }

  // Write new hook
  writeFileSync(precommitPath, hookScript);
  chmodSync(precommitPath, '755');
  console.log('');
  console.log(`Git pre-commit hook installed at ${precommitPath}`);
  console.log('  Warns on file claim conflicts when committing');
}
