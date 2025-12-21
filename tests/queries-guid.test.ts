import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

vi.mock('../src/core/guid.ts', async () => {
  const actual = await vi.importActual<typeof import('../src/core/guid.ts')>('../src/core/guid.ts');
  return {
    ...actual,
    generateGuid: vi.fn(actual.generateGuid),
  };
});

import Database from 'better-sqlite3';
import { initSchema } from '../src/db/schema.ts';
import { createAgent, createMessage, getMessage, getThread } from '../src/db/queries.ts';
import { generateGuid } from '../src/core/guid.ts';

describe('GUID queries', () => {
  let db: Database.Database;

  beforeEach(() => {
    db = new Database(':memory:');
    initSchema(db);

    const now = Math.floor(Date.now() / 1000);
    createAgent(db, {
      agent_id: 'alice.1',
      status: null,
      purpose: null,
      registered_at: now,
      last_seen: now,
    });
  });

  afterEach(() => {
    db.close();
  });

  it('should look up messages by GUID', () => {
    const msg = createMessage(db, {
      from_agent: 'alice.1',
      body: 'hello',
      mentions: [],
    });

    const found = getMessage(db, msg.id);
    expect(found?.id).toBe(msg.id);
    expect(found?.body).toBe('hello');
  });

  it('should return thread with parent first', () => {
    const root = createMessage(db, {
      ts: 1,
      from_agent: 'alice.1',
      body: 'root',
      mentions: [],
    });
    const reply1 = createMessage(db, {
      ts: 2,
      from_agent: 'alice.1',
      body: 'reply-1',
      mentions: [],
      reply_to: root.id,
    });
    const reply2 = createMessage(db, {
      ts: 3,
      from_agent: 'alice.1',
      body: 'reply-2',
      mentions: [],
      reply_to: root.id,
    });

    const thread = getThread(db, root.id);
    expect(thread.map(msg => msg.id)).toEqual([root.id, reply1.id, reply2.id]);
  });

  it('should retry GUID generation on collision', () => {
    const dup = 'msg-dupEabcd';
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run(dup, 1, null, 'alice.1', 'existing', '[]', 'agent', null, null, null);

    const guidMock = vi.mocked(generateGuid);
    guidMock
      .mockImplementationOnce(() => dup)
      .mockImplementationOnce(() => dup)
      .mockImplementationOnce(() => 'msg-uniqAbcd');

    const msg = createMessage(db, {
      from_agent: 'alice.1',
      body: 'new',
      mentions: [],
    });

    expect(msg.id).toBe('msg-uniqAbcd');
  });
});
