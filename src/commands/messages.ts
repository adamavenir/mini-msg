import { Command } from 'commander';
import { getContext, handleError } from './shared.js';
import { getMessages, getAgentBases } from '../db/queries.js';
import { formatMessage, getProjectName } from './format.js';

export function messagesAction(options: any, cmd: Command): void {
  try {
    const { db, project, jsonMode } = getContext(cmd);
    const projectName = getProjectName(project);
    const agentBases = getAgentBases(db);

    const messages = fetchMessages(db, options);

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

    db.close();
  } catch (error) {
    handleError(error);
  }
}

function fetchMessages(db: any, options: any): any[] {
  let limit: number | undefined;
  let since: string | undefined;

  if (options.all) {
    limit = undefined;
    since = undefined;
  } else if (options.since) {
    limit = undefined;
    since = options.since.replace(/^@?#/, '');
  } else {
    limit = parseInt(options.last || '20', 10);
  }

  return getMessages(db, { limit, since });
}
