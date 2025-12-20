import Database from 'better-sqlite3';
import { Message, MessageCursor } from '../types.js';
import { getMessages, createMessage } from '../db/queries.js';
import { extractMentions } from '../core/mentions.js';

export interface ChatSession {
  username: string;
  projectName: string;
  db: Database.Database;
  lastMessageCursor: MessageCursor | null;
}

export function createSession(opts: {
  db: Database.Database;
  projectName: string;
  username: string;
  lastMessageCursor?: MessageCursor | null;
}): ChatSession {
  return {
    db: opts.db,
    projectName: opts.projectName,
    username: opts.username,
    lastMessageCursor: opts.lastMessageCursor ?? null,
  };
}

export function pollNewMessages(session: ChatSession): Message[] {
  const messages = getMessages(session.db, {
    since: session.lastMessageCursor ?? undefined,
  });
  if (messages.length > 0) {
    const last = messages[messages.length - 1];
    session.lastMessageCursor = { guid: last.id, ts: last.ts };
  }
  return messages;
}

export function sendUserMessage(session: ChatSession, body: string): Message {
  const mentions = extractMentions(body, session.db);
  return createMessage(session.db, {
    from_agent: session.username,
    body,
    mentions,
    type: 'user',
  });
}
