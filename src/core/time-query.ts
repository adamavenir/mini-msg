import type Database from 'better-sqlite3';
import type { MessageCursor } from '../types.js';

const RELATIVE_RE = /^(\d+)(m|h|d|w)$/i;

function toCursorForTime(ts: number, mode: 'since' | 'before'): MessageCursor {
  const guid = mode === 'since' ? 'zzzzzzzz' : '';
  return { ts, guid };
}

function parseRelativeTime(value: string): Date | null {
  const match = value.match(RELATIVE_RE);
  if (!match) return null;

  const amount = parseInt(match[1], 10);
  const unit = match[2].toLowerCase();
  const seconds = (() => {
    switch (unit) {
      case 'm':
        return amount * 60;
      case 'h':
        return amount * 3600;
      case 'd':
        return amount * 86400;
      case 'w':
        return amount * 604800;
      default:
        return 0;
    }
  })();

  return new Date(Date.now() - seconds * 1000);
}

function parseAbsoluteTime(value: string): Date | null {
  const lowered = value.toLowerCase();
  const now = new Date();
  if (lowered === 'today') {
    return new Date(now.getFullYear(), now.getMonth(), now.getDate());
  }
  if (lowered === 'yesterday') {
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    return new Date(today.getTime() - 86400000);
  }
  return null;
}

function resolveGuidCursor(db: Database.Database, expr: string): MessageCursor | null {
  const trimmed = expr.trim();
  if (!trimmed.startsWith('#') && !trimmed.startsWith('msg-')) {
    return null;
  }

  let raw = trimmed;
  if (raw.startsWith('#')) {
    raw = raw.slice(1);
  }

  if (raw.startsWith('msg-')) {
    const row = db.prepare('SELECT guid, ts FROM mm_messages WHERE guid = ?')
      .get(raw) as { guid: string; ts: number } | undefined;
    if (!row) {
      throw new Error(`Message ${raw} not found`);
    }
    return { guid: row.guid, ts: row.ts };
  }

  if (raw.length < 2) {
    throw new Error(`GUID prefix too short: ${raw}`);
  }

  const like = `msg-${raw}%`;
  const rows = db.prepare(`
    SELECT guid, ts FROM mm_messages
    WHERE guid LIKE ?
    ORDER BY ts DESC, guid DESC
    LIMIT 5
  `).all(like) as { guid: string; ts: number }[];

  if (rows.length === 0) {
    throw new Error(`No message matches #${raw}`);
  }
  if (rows.length > 1) {
    const suggestions = rows.map(row => `#${row.guid.slice(4, 8)}`).join(', ');
    throw new Error(`Ambiguous #${raw}. Matches: ${suggestions}`);
  }

  return { guid: rows[0].guid, ts: rows[0].ts };
}

export function parseTimeExpression(
  db: Database.Database,
  expression: string,
  mode: 'since' | 'before'
): MessageCursor {
  const trimmed = expression.trim();

  const guidCursor = resolveGuidCursor(db, trimmed);
  if (guidCursor) {
    return guidCursor;
  }

  const absolute = parseAbsoluteTime(trimmed);
  if (absolute) {
    return toCursorForTime(Math.floor(absolute.getTime() / 1000), mode);
  }

  const relative = parseRelativeTime(trimmed);
  if (relative) {
    return toCursorForTime(Math.floor(relative.getTime() / 1000), mode);
  }

  throw new Error(`Invalid time expression: ${expression}`);
}
