import { Command } from 'commander';
import { execSync } from 'child_process';
import fs from 'fs';
import path from 'path';
import { getContext, handleError } from './shared.js';
import { readMessages, rebuildDatabaseFromJsonl } from '../db/jsonl.js';

export function pruneCommand(): Command {
  return new Command('prune')
    .description('Archive old messages with cold storage guardrails')
    .option('--keep <n>', 'number of recent messages to keep', '100')
    .option('--all', 'delete history.jsonl before pruning')
    .action((options, cmd) => {
      try {
        const { db, project, jsonMode } = getContext(cmd);

        // Guardrail: clean working tree for .mm/
        const status = execSync('git status --porcelain .mm/', {
          cwd: project.root,
          encoding: 'utf8',
        }).trim();
        if (status.length > 0) {
          throw new Error('Uncommitted changes in .mm/. Commit first.');
        }

        // Guardrail: ensure branch is synced with upstream (if upstream exists)
        let hasUpstream = false;
        try {
          execSync('git rev-parse --abbrev-ref --symbolic-full-name @{u}', {
            cwd: project.root,
            stdio: 'ignore',
          });
          hasUpstream = true;
        } catch {
          hasUpstream = false;
        }

        if (hasUpstream) {
          const ahead = parseInt(execSync('git rev-list --count @{u}..HEAD', {
            cwd: project.root,
            encoding: 'utf8',
          }).trim(), 10);
          const behind = parseInt(execSync('git rev-list --count HEAD..@{u}', {
            cwd: project.root,
            encoding: 'utf8',
          }).trim(), 10);

          if (ahead > 0 || behind > 0) {
            throw new Error('Branch not synced. Push/pull first.');
          }
        }

        const keep = parseInt(options.keep, 10);
        if (!Number.isInteger(keep) || keep < 0) {
          throw new Error(`Invalid --keep value: ${options.keep}`);
        }

        const mmDir = path.join(project.root, '.mm');
        const messagesPath = path.join(mmDir, 'messages.jsonl');
        const historyPath = path.join(mmDir, 'history.jsonl');

        if (options.all && fs.existsSync(historyPath)) {
          fs.unlinkSync(historyPath);
        }

        if (!options.all && fs.existsSync(messagesPath)) {
          const contents = fs.readFileSync(messagesPath, 'utf8');
          if (contents.trim().length > 0) {
            fs.appendFileSync(historyPath, contents, 'utf8');
          }
        }

        const messages = readMessages(project.dbPath);
        const kept = keep > 0 ? messages.slice(-keep) : [];

        const lines = kept.map(record => JSON.stringify(record)).join('\n');
        const output = lines.length > 0 ? lines + '\n' : '';
        fs.writeFileSync(messagesPath, output, 'utf8');

        rebuildDatabaseFromJsonl(db, project.dbPath);

        if (jsonMode) {
          console.log(JSON.stringify({
            kept: kept.length,
            archived: options.all ? 0 : messages.length,
            history: options.all ? null : historyPath,
          }, null, 2));
        } else {
          if (options.all) {
            console.log(`Pruned to last ${kept.length} messages. history.jsonl cleared.`);
          } else {
            console.log(`Pruned to last ${kept.length} messages. Archived to history.jsonl`);
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
