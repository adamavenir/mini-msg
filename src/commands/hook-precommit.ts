import { Command } from 'commander';
import { execSync } from 'child_process';
import { discoverProject, openDatabase } from '../core/project.js';
import { findConflictingFileClaims, getConfig } from '../db/queries.js';

export function hookPrecommitCommand(): Command {
  return new Command('hook-precommit')
    .description('Git pre-commit hook for file claim conflict detection')
    .action(() => {
      try {
        // Get current agent from environment
        const agentId = process.env.MM_AGENT_ID;

        // Try to discover mm project
        let project;
        try {
          project = discoverProject();
        } catch {
          // Not in an mm project, skip silently
          process.exit(0);
        }

        const db = openDatabase(project);

        // Get staged files
        let stagedFiles: string[];
        try {
          const output = execSync('git diff --cached --name-only', {
            encoding: 'utf-8',
            stdio: ['pipe', 'pipe', 'pipe'],
          });
          stagedFiles = output.trim().split('\n').filter(f => f.length > 0);
        } catch {
          // Not in a git repo or git error, skip silently
          db.close();
          process.exit(0);
        }

        if (stagedFiles.length === 0) {
          db.close();
          process.exit(0);
        }

        // Find conflicts with other agents' claims
        const conflicts = findConflictingFileClaims(db, stagedFiles, agentId);

        if (conflicts.length === 0) {
          db.close();
          process.exit(0);
        }

        // Group conflicts by agent
        const byAgent = new Map<string, { pattern: string; files: string[] }[]>();
        for (const claim of conflicts) {
          const existing = byAgent.get(claim.agent_id) || [];
          // Find which staged files match this claim
          const matchingFiles = stagedFiles.filter(f => {
            // Simple glob matching - for now just check prefix and exact match
            if (claim.pattern.includes('*')) {
              // Use micromatch for proper glob matching
              const micromatch = require('micromatch');
              return micromatch.isMatch(f, claim.pattern);
            }
            return f === claim.pattern || f.startsWith(claim.pattern + '/');
          });
          existing.push({ pattern: claim.pattern, files: matchingFiles });
          byAgent.set(claim.agent_id, existing);
        }

        // Print warning
        console.error('\n⚠️  FILE CLAIM CONFLICTS DETECTED\n');
        console.error('The following staged files are claimed by other agents:\n');

        for (const [agent, claims] of byAgent) {
          console.error(`  @${agent}:`);
          for (const { pattern, files } of claims) {
            if (files.length > 0) {
              for (const file of files) {
                console.error(`    ${file} (claimed via ${pattern})`);
              }
            } else {
              console.error(`    pattern: ${pattern}`);
            }
          }
        }

        console.error('\nConsider coordinating with these agents before committing.');
        console.error('Use "mm claims" to see all active claims.\n');

        // Check if strict mode is enabled
        const strictMode = getConfig(db, 'precommit_strict') === 'true';

        db.close();

        if (strictMode) {
          console.error('Commit blocked (precommit_strict mode enabled).');
          console.error('Use "mm config precommit_strict false" to disable strict mode.\n');
          process.exit(1);
        } else {
          console.error('Proceeding with commit (advisory mode).');
          console.error('Use "mm config precommit_strict true" to block commits with conflicts.\n');
          process.exit(0);
        }
      } catch (error) {
        // On any error, don't block the commit
        console.error('mm hook-precommit error:', error);
        process.exit(0);
      }
    });
}
