import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  createAgent,
  createMessage,
  getMessages,
  getMessagesWithMention,
  getLastMessageCursor,
  parseMessageRow,
} from '../src/db/queries.ts';
import { initSchema } from '../src/db/schema.ts';
import type { Message, MessageRow } from '../src/types.ts';
import Database from 'better-sqlite3';
import fs from 'fs';
import path from 'path';
import os from 'os';

describe('message query functions', () => {
  let db: Database.Database;
  let tempDbPath: string;

  beforeEach(() => {
    const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mm-messages-test-'));
    tempDbPath = path.join(tempDir, 'test.db');
    db = new Database(tempDbPath);
    initSchema(db);

    // Create test agents
    const now = Math.floor(Date.now() / 1000);
    createAgent(db, {
      agent_id: 'alice.1',
      status: null,
      purpose: null,
      registered_at: now,
      last_seen: now,
    });
    createAgent(db, {
      agent_id: 'bob.1',
      status: null,
      purpose: null,
      registered_at: now,
      last_seen: now,
    });
  });

  afterEach(() => {
    db.close();
    const tempDir = path.dirname(tempDbPath);
    fs.rmSync(tempDir, { recursive: true, force: true });
  });

  describe('parseMessageRow', () => {
    it('should parse mentions JSON', () => {
      const row: MessageRow = {
        guid: 'msg-test1234',
        ts: 1234567890,
        channel_id: null,
        from_agent: 'alice.1',
        body: 'hello @bob.1',
        mentions: '["bob.1"]',
        type: 'agent',
        reply_to: null,
        edited_at: null,
        archived_at: null,
      };

      const message = parseMessageRow(row);

      expect(message.id).toBe('msg-test1234');
      expect(message.mentions).toEqual(['bob.1']);
    });

    it('should handle empty mentions array', () => {
      const row: MessageRow = {
        guid: 'msg-test5678',
        ts: 1234567890,
        channel_id: null,
        from_agent: 'alice.1',
        body: 'hello world',
        mentions: '[]',
        type: 'agent',
        reply_to: null,
        edited_at: null,
        archived_at: null,
      };

      const message = parseMessageRow(row);
      expect(message.mentions).toEqual([]);
    });
  });

  describe('createMessage', () => {
    it('should create a message with mentions', () => {
      const message = createMessage(db, {
        from_agent: 'alice.1',
        body: 'hello @bob.1',
        mentions: ['bob.1'],
      });

      expect(message.id).toMatch(/^msg-[0-9a-z]{8}$/);
      expect(message.from_agent).toBe('alice.1');
      expect(message.body).toBe('hello @bob.1');
      expect(message.mentions).toEqual(['bob.1']);
      expect(message.ts).toBeGreaterThan(0);
    });

    it('should generate unique message GUIDs', () => {
      const msg1 = createMessage(db, {
        from_agent: 'alice.1',
        body: 'first',
        mentions: [],
      });

      const msg2 = createMessage(db, {
        from_agent: 'bob.1',
        body: 'second',
        mentions: [],
      });

      expect(msg1.id).not.toBe(msg2.id);
    });

    it('should handle multiple mentions', () => {
      const message = createMessage(db, {
        from_agent: 'alice.1',
        body: 'hello @bob.1 and @all',
        mentions: ['bob.1', 'all'],
      });

      expect(message.mentions).toEqual(['bob.1', 'all']);
    });
  });

  describe('getMessages', () => {
    let msg1: Message;
    let msg2: Message;
    let msg3: Message;

    beforeEach(() => {
      msg1 = createMessage(db, {
        ts: 1,
        from_agent: 'alice.1',
        body: 'message 1',
        mentions: [],
      });
      msg2 = createMessage(db, {
        ts: 2,
        from_agent: 'bob.1',
        body: 'message 2',
        mentions: ['alice.1'],
      });
      msg3 = createMessage(db, {
        ts: 3,
        from_agent: 'alice.1',
        body: 'message 3',
        mentions: [],
      });
    });

    it('should return all messages in chronological order', () => {
      const messages = getMessages(db);

      expect(messages.length).toBe(3);
      expect(messages[0].body).toBe('message 1');
      expect(messages[1].body).toBe('message 2');
      expect(messages[2].body).toBe('message 3');
    });

    it('should limit results to last N messages', () => {
      const messages = getMessages(db, { limit: 2 });

      expect(messages.length).toBe(2);
      // When limiting without since/before, get LAST N messages
      expect(messages[0].body).toBe('message 2');
      expect(messages[1].body).toBe('message 3');
    });

    it('should filter by since', () => {
      const messages = getMessages(db, { since: msg1.id });

      expect(messages.length).toBe(2);
      expect(messages[0].body).toBe('message 2');
      expect(messages[1].body).toBe('message 3');
    });

    it('should filter by before', () => {
      const messages = getMessages(db, { before: msg3.id });

      expect(messages.length).toBe(2);
      expect(messages[0].body).toBe('message 1');
      expect(messages[1].body).toBe('message 2');
    });

    it('should combine since and limit', () => {
      const messages = getMessages(db, { since: msg1.id, limit: 1 });

      expect(messages.length).toBe(1);
      expect(messages[0].body).toBe('message 2');
    });
  });

  describe('getMessagesWithMention', () => {
    let msg1: Message;
    let msg2: Message;
    let msg3: Message;
    let msg4: Message;

    beforeEach(() => {
      msg1 = createMessage(db, {
        ts: 1,
        from_agent: 'alice.1',
        body: 'hello @bob.1',
        mentions: ['bob.1'],
      });
      msg2 = createMessage(db, {
        ts: 2,
        from_agent: 'bob.1',
        body: 'hi @alice.1',
        mentions: ['alice.1'],
      });
      msg3 = createMessage(db, {
        ts: 3,
        from_agent: 'alice.1',
        body: 'no mentions here',
        mentions: [],
      });
      msg4 = createMessage(db, {
        ts: 4,
        from_agent: 'bob.1',
        body: 'broadcast @all',
        mentions: ['all'],
      });
    });

    it('should find messages mentioning exact agent', () => {
      const messages = getMessagesWithMention(db, 'bob.1');

      expect(messages.length).toBe(2); // @bob.1 and @all
      expect(messages.map(m => m.id)).toContain(msg1.id);
      expect(messages.map(m => m.id)).toContain(msg4.id);
    });

    it('should find messages with @all broadcasts', () => {
      const messages = getMessagesWithMention(db, 'alice.1');

      expect(messages.length).toBe(2); // @alice.1 and @all
      expect(messages.map(m => m.id)).toContain(msg2.id);
      expect(messages.map(m => m.id)).toContain(msg4.id);
    });

    it('should support prefix matching', () => {
      // Create an agent with qualified name
      const now = Math.floor(Date.now() / 1000);
      createAgent(db, {
        agent_id: 'alice.frontend.1',
        status: null,
        purpose: null,
        registered_at: now,
        last_seen: now,
      });

      createMessage(db, {
        from_agent: 'bob.1',
        body: 'message to @alice.frontend.1',
        mentions: ['alice.frontend.1'],
      });

      // Prefix "alice" should match alice.1 and alice.frontend.1
      const aliceMessages = getMessagesWithMention(db, 'alice');
      expect(aliceMessages.length).toBeGreaterThan(0);

      // Prefix "alice.frontend" should match alice.frontend.1
      const frontendMessages = getMessagesWithMention(db, 'alice.frontend');
      expect(frontendMessages.length).toBe(2); // alice.frontend.1 message + @all
      expect(frontendMessages.map(m => m.body)).toContain('message to @alice.frontend.1');
    });

    it('should support limit option', () => {
      const messages = getMessagesWithMention(db, 'alice.1', { limit: 1 });

      expect(messages.length).toBe(1);
    });

    it('should support since option', () => {
      const messages = getMessagesWithMention(db, 'bob.1', { since: msg1.id });

      // Should get messages after msg1
      // Message 1 mentions bob.1, but since: msg1 means skip it
      // Message 4 mentions @all, which matches everyone including bob.1
      expect(messages.length).toBe(1);
      expect(messages[0].id).toBe(msg4.id); // Only the @all broadcast
    });

    it('should return empty array for no matches', () => {
      const messages = getMessagesWithMention(db, 'charlie');

      expect(messages.length).toBe(1); // Only @all
      expect(messages[0].body).toBe('broadcast @all');
    });
  });

  describe('getLastMessageCursor', () => {
    it('should return null when no messages exist', () => {
      const lastCursor = getLastMessageCursor(db);
      expect(lastCursor).toBeNull();
    });

    it('should return the latest message cursor', () => {
      createMessage(db, {
        ts: 1,
        from_agent: 'alice.1',
        body: 'first',
        mentions: [],
      });
      createMessage(db, {
        ts: 2,
        from_agent: 'bob.1',
        body: 'second',
        mentions: [],
      });
      const lastMessage = createMessage(db, {
        ts: 3,
        from_agent: 'alice.1',
        body: 'third',
        mentions: [],
      });

      const lastCursor = getLastMessageCursor(db);
      expect(lastCursor).toEqual({ guid: lastMessage.id, ts: lastMessage.ts });
    });
  });
});
