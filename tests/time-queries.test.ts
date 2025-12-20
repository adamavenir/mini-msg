import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import Database from 'better-sqlite3';
import { initSchema } from '../src/db/schema.ts';
import { parseTimeExpression } from '../src/core/time-query.ts';

describe('parseTimeExpression edge cases', () => {
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

  it('resolves full GUID references', () => {
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-xyzx1111', 321, null, 'alice', 'hello', '[]', 'agent', null, null, null);

    const cursor = parseTimeExpression(db, 'msg-xyzx1111', 'since');
    expect(cursor.guid).toBe('msg-xyzx1111');
    expect(cursor.ts).toBe(321);
  });

  it('throws on ambiguous GUID prefixes with suggestions', () => {
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-abcd1111', 100, null, 'alice', 'one', '[]', 'agent', null, null, null);
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-abce2222', 101, null, 'alice', 'two', '[]', 'agent', null, null, null);

    expect(() => parseTimeExpression(db, '#abc', 'since'))
      .toThrow('Ambiguous #abc. Matches: #abce, #abcd');
  });

  it('throws when no GUID match exists', () => {
    expect(() => parseTimeExpression(db, '#zz', 'since'))
      .toThrow('No message matches #zz');
  });

  it('throws for prefixes shorter than two characters', () => {
    expect(() => parseTimeExpression(db, '#a', 'since'))
      .toThrow('GUID prefix too short');
  });

  it('parses multi-day relative times', () => {
    const cursor = parseTimeExpression(db, '2d', 'since');
    expect(cursor.guid).toBe('zzzzzzzz');
    expect(cursor.ts).toBe(Math.floor((Date.now() - 2 * 86400 * 1000) / 1000));
  });

  it('throws on invalid time expressions', () => {
    expect(() => parseTimeExpression(db, 'tomorrow', 'since'))
      .toThrow('Invalid time expression: tomorrow');
  });
});
