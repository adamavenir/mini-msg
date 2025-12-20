import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getMessage, getThread, getReplyCount } from '../db/queries.js';
import { formatMessage, getProjectName } from './format.js';
import { getAgentBases } from '../db/queries.js';

export function threadCommand(): Command {
  return new Command('thread')
    .description('View a message thread (parent + replies)')
    .argument('<id>', 'message GUID')
    .action(async (id: string, options, cmd) => {
      try {
        const { db, project, jsonMode } = getContext(cmd);
        const messageId = id.trim().replace(/^#/, '');

        const parentMsg = getMessage(db, messageId);
        if (!parentMsg) {
          throw new Error(`Message ${messageId} not found`);
        }

        // Get thread (parent + replies)
        const thread = getThread(db, messageId);
        const projectName = getProjectName(project);
        const agentBases = getAgentBases(db);

        if (jsonMode) {
          console.log(JSON.stringify({
            parent_id: messageId,
            messages: thread.map(m => ({
              id: m.id,
              from_agent: m.from_agent,
              body: m.body,
              reply_to: m.reply_to,
              ts: m.ts,
            })),
          }));
        } else {
          const replyCount = thread.length - 1; // Exclude parent
          console.log(`Thread #${messageId} (${replyCount} ${replyCount === 1 ? 'reply' : 'replies'}):\n`);

          for (const msg of thread) {
            const isParent = msg.id === messageId;
            const prefix = isParent ? '' : '  ↳ ';
            console.log(prefix + formatMessage(msg, projectName, agentBases));
          }
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
