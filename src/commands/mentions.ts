import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getMessagesWithMention, markMessagesRead, getReadReceiptCount, getAgentBases } from '../db/queries.js';
import { resolveAgentRef } from '../core/context.js';
import { formatMessage, getProjectName } from './format.js';

export function mentionsAction(agentRef: string, options: any, cmd: Command): void {
  try {
    const { db, project, jsonMode, projectConfig } = getContext(cmd);
    const projectName = getProjectName(project);
    const agentBases = getAgentBases(db);

    // Normalize @alice -> alice
    const prefix = resolveAgentRef(agentRef, projectConfig ?? null);

    let limit: number | undefined;
    let since: string | undefined;
    let unreadOnly = true;

    if (options.all) {
      limit = undefined;
      since = undefined;
      unreadOnly = false;
    } else if (options.since) {
      limit = undefined;
      since = options.since.replace(/^@?#/, '');
      unreadOnly = false;
    } else {
      limit = parseInt(options.last || '20', 10);
    }

    const messages = getMessagesWithMention(db, prefix, {
      limit,
      since,
      unreadOnly,
      agentPrefix: prefix,
      includeArchived: !!options.archived
    });

    if (jsonMode) {
      console.log(JSON.stringify(messages));
    } else {
      if (messages.length === 0) {
        if (unreadOnly) {
          console.log(`No unread mentions of @${prefix}`);
        } else {
          console.log(`No mentions of @${prefix}`);
        }
      } else {
        if (unreadOnly) {
          console.log(`Unread mentions of @${prefix}:`);
        } else {
          console.log(`Messages mentioning @${prefix}:`);
        }

        for (const msg of messages) {
          const readCount = getReadReceiptCount(db, msg.id);
          let formattedMsg = formatMessage(msg, projectName, agentBases);

          if (readCount > 0) {
            // Remove trailing newline if present, add read count, then re-add newline
            formattedMsg = formattedMsg.trimEnd();
            formattedMsg += ` [✓${readCount}]`;
          }

          console.log(formattedMsg);
        }
      }
    }

    // Mark displayed messages as read
    if (messages.length > 0) {
      const messageIds = messages.map(m => m.id);
      markMessagesRead(db, messageIds, prefix);
    }

    db.close();
  } catch (error) {
    handleError(error);
  }
}
