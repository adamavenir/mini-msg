import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getMessages, getMessagesWithMention, getFilter, markMessagesRead, getAgentBases } from '../db/queries.js';
import { formatMessage, getProjectName } from './format.js';
import { resolveAgentRef } from '../core/context.js';
import { parseTimeExpression } from '../core/time-query.js';
import type { Filter } from '../types.js';

export function getCommand(): Command {
  return new Command('get')
    .description('Get messages (combined view or simple query)')
    .argument('[agent]', 'agent ID for combined room + @mentions view')
    .option('--last <n>', 'show last N messages')
    .option('--since <time|guid>', 'show messages after time or GUID')
    .option('--before <time|guid>', 'show messages before time or GUID')
    .option('--from <time|guid>', 'range start (time or GUID)')
    .option('--to <time|guid>', 'range end (time or GUID)')
    .option('--all', 'show all messages')
    .option('--room <n>', 'number of room messages in combined view', '10')
    .option('--mentions <n>', 'number of @mentions in combined view', '3')
    .option('--unfiltered', 'bypass saved filter, show all messages')
    .option('--archived', 'include archived messages')
    .allowUnknownOption(false)
    .action((agentId: string | undefined, options, cmd) => {
      try {
        // Check if we're in query mode (explicit query params)
        const isQueryMode = !!(options.last || options.since || options.before || options.from || options.to || options.all);

        const { db, project, jsonMode, projectConfig } = getContext(cmd);
        const projectName = getProjectName(project);
        const agentBases = getAgentBases(db);
        const resolvedAgentId = agentId ? resolveAgentRef(agentId, projectConfig ?? null) : undefined;

        // Load filter if agent is provided and not bypassed
        let filter: Filter | null = null;
        if (resolvedAgentId && !options.unfiltered) {
          filter = getFilter(db, resolvedAgentId);
        }

        if (isQueryMode) {
          // Simple query mode
          let limit: number | undefined;
          let since;
          let before;

          if (options.all) {
            limit = undefined;
            since = undefined;
            before = undefined;
          } else if (options.since || options.before || options.from || options.to) {
            if (options.since && options.from) {
              throw new Error('Use --since or --from, not both');
            }
            if (options.before && options.to) {
              throw new Error('Use --before or --to, not both');
            }
            const sinceValue = options.from ?? options.since;
            const beforeValue = options.to ?? options.before;
            since = sinceValue ? parseTimeExpression(db, sinceValue, 'since') : undefined;
            before = beforeValue ? parseTimeExpression(db, beforeValue, 'before') : undefined;
          } else {
            limit = parseInt(options.last, 10);
          }

          const messages = getMessages(db, {
            limit,
            since,
            before,
            filter,
            includeArchived: !!options.archived
          });

          if (jsonMode) {
            console.log(JSON.stringify(messages));
          } else {
            if (messages.length === 0) {
              console.log('No messages');
            } else {
              for (const msg of messages) {
                console.log(formatMessage(msg, projectName, agentBases));
              }
            }
          }
        } else if (resolvedAgentId) {
          // Combined view mode
          const roomLimit = parseInt(options.room, 10);
          const mentionsLimit = parseInt(options.mentions, 10);

          // Get latest room messages (with filter applied)
          const roomMessages = getMessages(db, {
            limit: roomLimit,
            filter,
            includeArchived: !!options.archived
          });

          // Get mentions for this agent
          // Strip version if provided (alice.1 -> alice) for broader matching
          const agentBase = resolvedAgentId.includes('.')
            ? resolvedAgentId.substring(0, resolvedAgentId.lastIndexOf('.'))
            : resolvedAgentId;
          const mentionMessages = getMessagesWithMention(db, agentBase, {
            limit: mentionsLimit + roomLimit,
            includeArchived: !!options.archived
          });

          // Filter out mentions that are already in room messages
          const roomIds = new Set(roomMessages.map(m => m.id));
          let filteredMentions = mentionMessages.filter(m => !roomIds.has(m.id));

          // Limit to requested count
          filteredMentions = filteredMentions.slice(0, mentionsLimit);

          // Mark mentions as read
          if (filteredMentions.length > 0) {
            const mentionIds = filteredMentions.map(m => m.id);
            markMessagesRead(db, mentionIds, agentBase);
          }

          if (jsonMode) {
            console.log(JSON.stringify({
              project: projectName,
              room_messages: roomMessages,
              mentions: filteredMentions,
            }));
          } else {
            // Room messages
            if (roomMessages.length === 0) {
              console.log('ROOM: (no messages yet)');
            } else {
              console.log('ROOM:');
              for (const msg of roomMessages) {
                console.log(formatMessage(msg, projectName, agentBases));
              }
            }

            console.log('');
            console.log('---');
            console.log('');

            // @mentions
            if (filteredMentions.length === 0) {
              console.log(`@${agentBase}: (no additional mentions)`);
            } else {
              console.log(`@${agentBase}:`);
              for (const msg of filteredMentions) {
                console.log(formatMessage(msg, projectName, agentBases));
              }
            }

            console.log('');
            console.log('---');
            console.log(`More: mm get --last 50 | mm @${agentBase} --all | mm get --since <guid>`);
          }

          // Mark displayed mentions as read
          if (filteredMentions.length > 0) {
            const mentionIds = filteredMentions.map(m => m.id);
            markMessagesRead(db, mentionIds, agentBase);
          }
        } else {
          // No agent, no query params - show help
          console.error('Usage: mm get <agent>        Combined room + @mentions view');
          console.error('       mm get --last <n>     Last N messages');
          console.error('       mm get --since <guid> Messages after GUID');
          console.error('       mm get --all          All messages');
          process.exit(1);
        }

        db.close();
      } catch (error) {
        handleError(error);
      }
    });
}
