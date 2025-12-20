import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import Database from 'better-sqlite3';
import { initSchema } from '../src/db/schema.ts';
import { resolveReplyReference } from '../src/chat/reply.ts';

describe('reply prefix resolution', () => {
  let db: Database.Database;

  beforeEach(() => {
    db = new Database(':memory:');
    initSchema(db);
  });

  afterEach(() => {
    db.close();
  });

  it('accepts full GUIDs with #msg- prefix', () => {
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-abcd1234', 1, null, 'alice', 'root', '[]', 'agent', null, null, null);

    const result = resolveReplyReference(db, '#msg-abcd1234 replying');
    expect(result.kind).toBe('resolved');
    if (result.kind === 'resolved') {
      expect(result.reply_to).toBe('msg-abcd1234');
      expect(result.body).toBe('replying');
    }
  });

  it('ignores prefixes shorter than two characters', () => {
    const result = resolveReplyReference(db, '#a too-short');
    expect(result.kind).toBe('none');
    if (result.kind === 'none') {
      expect(result.body).toBe('#a too-short');
    }
  });

  it('resolves GUID prefixes with lowercase matches', () => {
    db.prepare(`
      INSERT INTO mm_messages (guid, ts, channel_id, from_agent, body, mentions, type, reply_to, edited_at, archived_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `).run('msg-abcd9999', 5, null, 'alice', 'case-test', '[]', 'agent', null, null, null);

    const result = resolveReplyReference(db, '#abcd ping');
    expect(result.kind).toBe('resolved');
    if (result.kind === 'resolved') {
      expect(result.reply_to).toBe('msg-abcd9999');
    }
  });
});
