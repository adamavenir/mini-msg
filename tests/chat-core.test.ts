import { describe, test, expect, beforeEach } from 'vitest';
import Database from 'better-sqlite3';
import { initSchema } from '../src/db/schema.js';
import { createAgent, createMessage } from '../src/db/queries.js';
import { createSession, pollNewMessages, sendUserMessage } from '../src/chat/core.js';

describe('Chat Core', () => {
  let db: Database.Database;

  beforeEach(() => {
    db = new Database(':memory:');
    initSchema(db);
  });

  describe('createSession', () => {
    test('creates session with provided lastMessageCursor', () => {
      const session = createSession({
        db,
        projectName: 'test-project',
        username: 'alice.1',
        lastMessageCursor: { guid: 'msg-test1234', ts: 42 },
      });

      expect(session.username).toBe('alice.1');
      expect(session.projectName).toBe('test-project');
      expect(session.db).toBe(db);
      expect(session.lastMessageCursor).toEqual({ guid: 'msg-test1234', ts: 42 });
    });

    test('defaults lastMessageCursor to null', () => {
      const session = createSession({
        db,
        projectName: 'test-project',
        username: 'alice.1',
      });

      expect(session.lastMessageCursor).toBeNull();
    });
  });

  describe('pollNewMessages', () => {
    test('returns empty array when no new messages', () => {
      const session = createSession({
        db,
        projectName: 'test',
        username: 'alice.1',
      });

      const messages = pollNewMessages(session);
      expect(messages).toEqual([]);
      expect(session.lastMessageCursor).toBeNull();
    });

    test('returns new messages and updates lastMessageCursor', () => {
      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: Date.now() / 1000,
        last_seen: Date.now() / 1000,
      });

      const msg1 = createMessage(db, {
        ts: 1,
        from_agent: 'alice.1',
        body: 'First message',
        mentions: [],
        type: 'agent',
      });

      const msg2 = createMessage(db, {
        ts: 2,
        from_agent: 'alice.1',
        body: 'Second message',
        mentions: [],
        type: 'agent',
      });

      const session = createSession({
        db,
        projectName: 'test',
        username: 'alice.1',
      });

      const messages = pollNewMessages(session);
      expect(messages).toHaveLength(2);
      expect(messages[0].id).toBe(msg1.id);
      expect(messages[1].id).toBe(msg2.id);
      expect(session.lastMessageCursor).toEqual({ guid: msg2.id, ts: msg2.ts });
    });

    test('returns only messages after lastMessageCursor', () => {
      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: Date.now() / 1000,
        last_seen: Date.now() / 1000,
      });

      const msg1 = createMessage(db, {
        ts: 1,
        from_agent: 'alice.1',
        body: 'First',
        mentions: [],
        type: 'agent',
      });

      const session = createSession({
        db,
        projectName: 'test',
        username: 'alice.1',
        lastMessageCursor: { guid: msg1.id, ts: msg1.ts },
      });

      const msg2 = createMessage(db, {
        ts: 2,
        from_agent: 'alice.1',
        body: 'Second',
        mentions: [],
        type: 'agent',
      });

      const messages = pollNewMessages(session);
      expect(messages).toHaveLength(1);
      expect(messages[0].id).toBe(msg2.id);
      expect(session.lastMessageCursor).toEqual({ guid: msg2.id, ts: msg2.ts });
    });

    test('sequential polls return only new messages', () => {
      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: Date.now() / 1000,
        last_seen: Date.now() / 1000,
      });

      const session = createSession({
        db,
        projectName: 'test',
        username: 'alice.1',
      });

      createMessage(db, {
        ts: 1,
        from_agent: 'alice.1',
        body: 'First',
        mentions: [],
        type: 'agent',
      });

      const firstPoll = pollNewMessages(session);
      expect(firstPoll).toHaveLength(1);

      const secondPoll = pollNewMessages(session);
      expect(secondPoll).toHaveLength(0);

      createMessage(db, {
        ts: 2,
        from_agent: 'alice.1',
        body: 'Second',
        mentions: [],
        type: 'agent',
      });

      const thirdPoll = pollNewMessages(session);
      expect(thirdPoll).toHaveLength(1);
      expect(thirdPoll[0].body).toBe('Second');
    });
  });

  describe('sendUserMessage', () => {
    test('creates message with type=user', () => {
      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: Date.now() / 1000,
        last_seen: Date.now() / 1000,
      });

      const session = createSession({
        db,
        projectName: 'test',
        username: 'alice.1',
      });

      const message = sendUserMessage(session, 'Hello world');

      expect(message.from_agent).toBe('alice.1');
      expect(message.body).toBe('Hello world');
      expect(message.type).toBe('user');
      expect(message.mentions).toEqual([]);
      expect(message.id).toMatch(/^msg-[0-9a-z]{8}$/);
    });

    test('extracts mentions from body', () => {
      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: Date.now() / 1000,
        last_seen: Date.now() / 1000,
      });

      const session = createSession({
        db,
        projectName: 'test',
        username: 'alice.1',
      });

      const message = sendUserMessage(session, '@bob.2 @carol.3 hey there');

      expect(message.mentions).toEqual(['bob.2', 'carol.3']);
    });

    test('returns created message for immediate display', () => {
      createAgent(db, {
        agent_id: 'alice.1',
        status: null,
        purpose: null,
        registered_at: Date.now() / 1000,
        last_seen: Date.now() / 1000,
      });

      const session = createSession({
        db,
        projectName: 'test',
        username: 'alice.1',
      });

      const message = sendUserMessage(session, 'Test message');

      expect(message).toBeDefined();
      expect(message.id).toMatch(/^msg-[0-9a-z]{8}$/);
      expect(message.ts).toBeGreaterThan(0);
    });
  });
});
