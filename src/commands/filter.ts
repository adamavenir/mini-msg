import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getFilter, setFilter, clearFilter } from '../db/queries.js';

export function filterCommand(): Command {
  const cmd = new Command('filter')
    .description('Manage message filter preferences');

  cmd.command('set')
    .description('Set filter preferences')
    .option('--mentions <pattern>', 'Mention pattern (e.g., "claude" or "claude,pm")')
    .action((options, cmd) => {
      try {
        const { db } = getContext(cmd);
        const agentId = process.env.MM_AGENT_ID;
        if (!agentId) {
          throw new Error('MM_AGENT_ID not set. Run mm new first.');
        }

        setFilter(db, {
          agent_id: agentId,
          mentions_pattern: options.mentions ?? null,
        });

        console.log('Filter updated');
        db.close();
      } catch (error) {
        handleError(error);
      }
    });

  cmd.command('show')
    .description('Show current filter')
    .action((options, cmd) => {
      try {
        const { db } = getContext(cmd);
        const agentId = process.env.MM_AGENT_ID;
        if (!agentId) {
          throw new Error('MM_AGENT_ID not set');
        }

        const filter = getFilter(db, agentId);
        if (!filter) {
          console.log('No filter set');
        } else {
          console.log('Current filter:');
          if (filter.mentions_pattern) console.log(`  Mentions: ${filter.mentions_pattern}`);
        }
        db.close();
      } catch (error) {
        handleError(error);
      }
    });

  cmd.command('clear')
    .description('Clear all filters')
    .action((options, cmd) => {
      try {
        const { db } = getContext(cmd);
        const agentId = process.env.MM_AGENT_ID;
        if (!agentId) {
          throw new Error('MM_AGENT_ID not set');
        }

        clearFilter(db, agentId);
        console.log('Filter cleared');
        db.close();
      } catch (error) {
        handleError(error);
      }
    });

  return cmd;
}
