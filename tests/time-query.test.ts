import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import Database from 'better-sqlite3';
import { initSchema } from '../src/db/schema.ts';
import { parseTimeExpression } from '../src/core/time-query.ts';

describe('parseTimeExpression', () => {
  let db: Database.Database;

  beforeEach(() => {
    db = new Database(':memory:');
    initSchema(db);
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-02T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
    db.close();
  });

  it('parses relative times', () => {
    const cursor = parseTimeExpression(db, '1h', 'since');
    expect(cursor.guid).toBe('zzzzzzzz');
    expect(cursor.ts).toBe(Math.floor((Date.now() - 3600 * 1000) / 1000));
  });

  it('parses today and yesterday', () => {
    const today = parseTimeExpression(db, 'today', 'before');
    const yesterday = parseTimeExpression(db, 'yesterday', 'before');
    const now = new Date();
    const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const startOfYesterday = new Date(startOfToday.getTime() - 86400000);
    expect(today.ts).toBe(Math.floor(startOfToday.getTime() / 1000));
    expect(yesterday.ts).toBe(Math.floor(startOfYesterday.getTime() / 1000));
  });

  it('resolves GUID prefixes', () => {
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-abcd1111', 123, null, 'alice.1', 'hello', '[]', 'agent', null, null, null);

    const cursor = parseTimeExpression(db, '#abcd', 'since');
    expect(cursor.guid).toBe('msg-abcd1111');
    expect(cursor.ts).toBe(123);
  });
});
