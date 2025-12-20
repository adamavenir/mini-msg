import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getAgentColor } from './format.js';

export function viewCommand(): Command {
  return new Command('view')
    .description('View full message without truncation')
    .argument('<id>', 'message GUID')
    .action((id: string, options, cmd) => {
      try {
        const { db } = getContext(cmd);
        const msgId = id.trim().replace(/^#/, '');

        const stmt = db.prepare('SELECT * FROM mm_messages WHERE guid = ?');
        const row = stmt.get(msgId) as any;

        if (!row) {
          console.log(`Message ${msgId} not found`);
          db.close();
          return;
        }

        const color = getAgentColor(row.from_agent, row.type);

        if (color) {
          console.log(`${color}Message #${row.guid} from @${row.from_agent}:\x1b[0m`);
          console.log(`${color}${row.body}\x1b[0m`);
        } else {
          console.log(`Message #${row.guid} from @${row.from_agent}:`);
          console.log(row.body);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
