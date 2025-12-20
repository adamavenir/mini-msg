import type Database from 'better-sqlite3';

export type ReplyMatch = {
  guid: string;
  ts: number;
  from_agent: string;
  body: string;
};

export type ReplyResolution =
  | { kind: 'none'; body: string }
  | { kind: 'resolved'; body: string; reply_to: string; match: ReplyMatch }
  | { kind: 'ambiguous'; body: string; prefix: string; matches: ReplyMatch[] };

const REPLY_PREFIX_RE = /^\s*#([A-Za-z0-9-]{2,})\b/;

function normalizePrefix(raw: string): string {
  if (raw.toLowerCase().startsWith('msg-')) {
    return raw.slice(4);
  }
  return raw;
}

export function resolveReplyReference(db: Database.Database, text: string): ReplyResolution {
  const match = text.match(REPLY_PREFIX_RE);
  if (!match) {
    return { kind: 'none', body: text };
  }

  const prefix = normalizePrefix(match[1]);
  if (prefix.length < 2) {
    return { kind: 'none', body: text };
  }

  const stripped = text.slice(match[0].length).trimStart();
  const rows = db.prepare(`
    SELECT guid, ts, from_agent, body
    FROM mm_messages
    WHERE guid LIKE ?
    ORDER BY ts DESC, guid DESC
    LIMIT 5
  `).all(`msg-${prefix}%`) as ReplyMatch[];

  if (rows.length === 0) {
    return { kind: 'none', body: text };
  }

  if (rows.length === 1) {
    return { kind: 'resolved', body: stripped, reply_to: rows[0].guid, match: rows[0] };
  }

  return { kind: 'ambiguous', body: stripped, prefix, matches: rows };
}
