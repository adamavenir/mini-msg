import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import {
  getMessages,
  getAgentBases,
  getLastMessageCursor,
} from '../db/queries.js';
import { formatMessage, getProjectName } from './format.js';
import type { MessageCursor } from '../types.js';

export function watchCommand(): Command {
  return new Command('watch')
    .description('Stream messages in real-time')
    .option('--last <n>', 'show last N messages before streaming', '10')
    .option('--archived', 'include archived messages')
    .action(async (options, cmd) => {
      try {
        const { db, project, jsonMode } = getContext(cmd);
        const projectName = getProjectName(project);
        const agentBases = getAgentBases(db);

        let lastMessageCursor: MessageCursor | null = null;

        const lastN = parseInt(options.last, 10);

        if (lastN === 0) {
          // Skip history, start from current
          lastMessageCursor = getLastMessageCursor(db);
          if (!jsonMode) {
            console.log('--- watching (Ctrl+C to stop) ---');
          }
        } else {
          // Show recent context first
          const recent = getMessages(db, {
            limit: lastN,
            includeArchived: !!options.archived
          });
          if (recent.length > 0) {
            if (jsonMode) {
              for (const msg of recent) {
                console.log(JSON.stringify(msg));
              }
            } else {
              for (const msg of recent) {
                console.log(formatMessage(msg, projectName, agentBases));
              }
              console.log('--- watching (Ctrl+C to stop) ---');
            }
            const last = recent[recent.length - 1];
            lastMessageCursor = { guid: last.id, ts: last.ts };
          } else if (!jsonMode) {
            console.log('--- watching (Ctrl+C to stop) ---');
          }
        }

        // Poll for new messages
        const pollInterval = 1000; // 1 second
        setInterval(() => {
          const newMessages = getMessages(db, {
            since: lastMessageCursor ?? undefined,
            includeArchived: !!options.archived
          });
          if (newMessages.length > 0) {
            for (const msg of newMessages) {
              if (jsonMode) {
                console.log(JSON.stringify(msg));
              } else {
                console.log(formatMessage(msg, projectName, agentBases));
              }
            }
            const last = newMessages[newMessages.length - 1];
            lastMessageCursor = { guid: last.id, ts: last.ts };
          }
        }, pollInterval);

        // Keep process alive
        process.stdin.resume();
      } catch (error) {
        handleError(error);
      }
    });
}
