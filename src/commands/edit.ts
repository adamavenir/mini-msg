import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { editMessage } from '../db/queries.js';
import { resolveAgentRef } from '../core/context.js';

export function editCommand(): Command {
  return new Command('edit')
    .description('Edit a message you posted')
    .requiredOption('--as <agent>', 'agent ID editing the message')
    .argument('<msgid>', 'message GUID to edit')
    .argument('<message>', 'new message text')
    .action(async (msgidStr: string, message: string, options, cmd) => {
      try {
        const { db, jsonMode, projectConfig } = getContext(cmd);
        const agentId = resolveAgentRef(options.as, projectConfig ?? null);
        const msgid = msgidStr.trim().replace(/^#/, '');

        editMessage(db, msgid, message, agentId);

        if (jsonMode) {
          console.log(JSON.stringify({ id: msgid, edited: true }));
        } else {
          console.log(`Edited message #${msgid}`);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
