import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import Database from 'better-sqlite3';
import { initSchema } from '../src/db/schema.ts';
import { resolveReplyReference } from '../src/chat/reply.ts';
import { createAgent } from '../src/db/queries.ts';

describe('resolveReplyReference', () => {
  let db: Database.Database;

  beforeEach(() => {
    db = new Database(':memory:');
    initSchema(db);

    const now = Math.floor(Date.now() / 1000);
    createAgent(db, {
      agent_id: 'alice.1',
      goal: null,
      bio: null,
      registered_at: now,
      last_seen: now,
    });
  });

  afterEach(() => {
    db.close();
  });

  it('returns none when no reply prefix is present', () => {
    const result = resolveReplyReference(db, 'hello world');
    expect(result.kind).toBe('none');
    if (result.kind === 'none') {
      expect(result.body).toBe('hello world');
    }
  });

  it('resolves to a single message match', () => {
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-abcd1111', 1, null, 'alice.1', 'root', '[]', 'agent', null, null, null);

    const result = resolveReplyReference(db, '#abcd replying');
    expect(result.kind).toBe('resolved');
    if (result.kind === 'resolved') {
      expect(result.reply_to).toBe('msg-abcd1111');
      expect(result.body).toBe('replying');
    }
  });

  it('returns ambiguous when multiple matches exist', () => {
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-abcd1111', 1, null, 'alice.1', 'root-1', '[]', 'agent', null, null, null);
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-abcd2222', 2, null, 'alice.1', 'root-2', '[]', 'agent', null, null, null);

    const result = resolveReplyReference(db, '#abcd another');
    expect(result.kind).toBe('ambiguous');
    if (result.kind === 'ambiguous') {
      expect(result.matches.length).toBe(2);
      expect(result.prefix).toBe('abcd');
    }
  });

  it('treats unknown prefixes as literal text', () => {
    const result = resolveReplyReference(db, '#zz missing');
    expect(result.kind).toBe('none');
    if (result.kind === 'none') {
      expect(result.body).toBe('#zz missing');
    }
  });
});
