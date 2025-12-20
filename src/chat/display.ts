import Database from 'better-sqlite3';
import { highlightMentions } from '../core/mentions.js';
import { getDisplayPrefixLength, getGuidPrefix } from '../core/guid.js';
import { getAgentColor } from '../commands/format.js';
import { getAgentBases } from '../db/queries.js';
import type { ChatDisplay, FormattedMessage } from './types.js';

const NO_COLOR = !!process.env.NO_COLOR;

const DIM = NO_COLOR ? '' : '\x1b[2m';
const BOLD = NO_COLOR ? '' : '\x1b[1m';
const ITALIC = NO_COLOR ? '' : '\x1b[3m';
const RESET = NO_COLOR ? '' : '\x1b[0m';
const USER_COLOR = NO_COLOR ? '' : '\x1b[38;5;249m';
const BLACK_TEXT = NO_COLOR ? '' : '\x1b[38;5;16m';
const WHITE_TEXT = NO_COLOR ? '' : '\x1b[38;5;231m';
const ISSUE_BG = NO_COLOR ? '' : '\x1b[48;5;17m';

const MAX_DISPLAY_LINES = 500;

function truncatePreview(body: string, maxLength: number = 50): string {
  const compact = body.replace(/\s+/g, ' ').trim();
  if (compact.length <= maxLength) {
    return compact;
  }
  return compact.slice(0, maxLength - 3) + '...';
}

/**
 * Extract color code from ANSI color sequence.
 * Expects format like '\x1b[38;5;111m' and extracts '111'.
 */
function extractColorCode(ansiColor: string): number | null {
  const match = ansiColor.match(/38;5;(\d+)/);
  return match ? parseInt(match[1], 10) : null;
}

/**
 * Convert ANSI 256 color code to RGB values.
 * Returns [r, g, b] where each component is 0-255.
 */
function colorCodeToRgb(code: number): [number, number, number] {
  // Standard colors (0-15) - simplified mapping
  if (code < 16) {
    const standard = [
      [0, 0, 0], [128, 0, 0], [0, 128, 0], [128, 128, 0],
      [0, 0, 128], [128, 0, 128], [0, 128, 128], [192, 192, 192],
      [128, 128, 128], [255, 0, 0], [0, 255, 0], [255, 255, 0],
      [0, 0, 255], [255, 0, 255], [0, 255, 255], [255, 255, 255]
    ];
    return standard[code] as [number, number, number];
  }

  // 6x6x6 RGB cube (16-231)
  if (code >= 16 && code <= 231) {
    const index = code - 16;
    const r = Math.floor(index / 36);
    const g = Math.floor((index % 36) / 6);
    const b = index % 6;

    // Map 0-5 to actual RGB values
    const toRgb = (val: number) => val === 0 ? 0 : 55 + val * 40;
    return [toRgb(r), toRgb(g), toRgb(b)];
  }

  // Grayscale (232-255)
  if (code >= 232 && code <= 255) {
    const gray = 8 + (code - 232) * 10;
    return [gray, gray, gray];
  }

  // Fallback
  return [128, 128, 128];
}

/**
 * Calculate luminance and return appropriate text color (black or white).
 * Uses standard luminance formula: 0.299*R + 0.587*G + 0.114*B
 */
function getContrastTextColor(bgColor: string): string {
  const code = extractColorCode(bgColor);
  if (code === null) return WHITE_TEXT;

  const [r, g, b] = colorCodeToRgb(code);
  const luminance = 0.299 * r + 0.587 * g + 0.114 * b;

  // If luminance > 128, background is light, use black text
  return luminance > 128 ? BLACK_TEXT : WHITE_TEXT;
}

/**
 * Unescape common shell escaping patterns.
 * Removes backslashes before special characters.
 */
function unescapeBody(body: string): string {
  return body.replace(/\\([!@#$%^&*()])/g, '$1');
}

/**
 * Highlight beads issue IDs with dark blue background and white text.
 * Matches pattern: #<prefix>-<alphanumeric> (e.g., #mm-123, #myproject-5ar)
 * @param body Message body text
 * @param senderColor The sender's color code (used to restore after each highlight)
 * @returns Body with highlighted issue IDs
 */
function highlightIssueIds(body: string, senderColor: string = ''): string {
  return body.replace(/#([a-z][a-z0-9]*)-([a-z0-9]+)\b/gi, (match) => {
    return `${ISSUE_BG}${WHITE_TEXT}${match}${RESET}${senderColor}`;
  });
}

/**
 * Colorize @mentions in a message body.
 * Validates against known agent bases when provided.
 * @param body Message body text
 * @param senderColor The sender's color code (used to reset after each mention)
 * @param agentBases Optional set of known agent base names for validation
 * @param colorMap Optional map from agent base to color index
 * @returns Body with colorized mentions
 */
function colorizeBody(body: string, senderColor: string, agentBases?: Set<string>, colorMap?: Map<string, number>): string {
  const unescaped = unescapeBody(body);
  return unescaped.replace(/@([a-z][a-z0-9.]*)/gi, (match, name) => {
    const lowercase = name.toLowerCase();

    // Always colorize @all
    if (lowercase === 'all') {
      const mentionColor = getAgentColor(name, 'agent', colorMap);
      if (mentionColor) {
        return `${mentionColor}${match}${senderColor}`;
      }
    }

    // Colorize if it has a dot (agent ID like alice.1)
    if (name.includes('.')) {
      const mentionColor = getAgentColor(name, 'agent', colorMap);
      if (mentionColor) {
        return `${mentionColor}${match}${senderColor}`;
      }
    }

    // For plain names, validate against agent bases if provided
    if (agentBases) {
      if (agentBases.has(lowercase)) {
        const mentionColor = getAgentColor(name, 'agent', colorMap);
        if (mentionColor) {
          return `${mentionColor}${match}${senderColor}`;
        }
      }

      // User mentions: not in agentBases, no dots, 3-15 chars
      if (!name.includes('.') && name.length >= 3 && name.length <= 15) {
        return `${BOLD}${match}${RESET}${senderColor}`;
      }
    } else {
      // No validation available - use fallback heuristic
      // Colorize names that are 3-15 chars (typical usernames)
      if (name.length >= 3 && name.length <= 15) {
        const mentionColor = getAgentColor(name, 'agent', colorMap);
        if (mentionColor) {
          return `${mentionColor}${match}${senderColor}`;
        }
      }
    }

    return match;
  });
}

/**
 * Truncate message body for display if it exceeds MAX_DISPLAY_LINES.
 * Full message remains in DB, but display shows first N lines with indicator.
 */
function truncateForDisplay(body: string, msgId: string): string {
  const lines = body.split('\n');
  if (lines.length <= MAX_DISPLAY_LINES) {
    return body;
  }

  const truncated = lines.slice(0, MAX_DISPLAY_LINES).join('\n');
  const remaining = lines.length - MAX_DISPLAY_LINES;
  return truncated + '\n' +
         `... (${remaining} more lines. Use '/view ${msgId}' to see full)`;
}

export class AnsiChatDisplay implements ChatDisplay {
  private db: Database.Database;
  private colorMap?: Map<string, number>;

  constructor(db: Database.Database, colorMap?: Map<string, number>) {
    this.db = db;
    this.colorMap = colorMap;
  }

  renderMessage(msg: FormattedMessage): void {
    // Get color for sender
    const color = msg.type === 'user'
      ? USER_COLOR  // Fixed color for all users
      : getAgentColor(msg.sender, msg.type, this.colorMap);

    // Get agent bases for mention validation
    const agentBases = getAgentBases(this.db);

    // Truncate and colorize message body
    // Apply @mention colorization first, then issue ID highlighting
    const truncatedBody = truncateForDisplay(msg.body, msg.id);
    const colorizedBody = color
      ? highlightIssueIds(colorizeBody(truncatedBody, color, agentBases, this.colorMap), color)
      : highlightIssueIds(unescapeBody(truncatedBody));

    // Format speaker line: 1 space + @sender + : + 1 space
    // Background is agent's color, text is bold black or white based on luminance
    const speakerContent = ` @${msg.sender}: `;

    let speakerLine: string;
    if (color) {
      const textColor = getContrastTextColor(color);
      // Convert foreground color (38;5;N) to background color (48;5;N)
      const bgColor = color.replace('38;5;', '48;5;');
      speakerLine = `${bgColor}${BOLD}${textColor}${speakerContent}${RESET}`;
    } else {
      speakerLine = `${BOLD}${speakerContent}${RESET}`;
    }

    const prefixLength = this.getPrefixLength();
    const idSuffix = `${DIM} #${getGuidPrefix(msg.id, prefixLength)}${RESET}`;
    const bodyLine = color ? `${color}${colorizedBody}${RESET}${idSuffix}` : `${colorizedBody}${idSuffix}`;
    const replyContext = msg.reply_to ? this.buildReplyContext(msg.reply_to, prefixLength) : '';
    const formatted = replyContext
      ? `${speakerLine}\n${replyContext}\n${bodyLine}`
      : `${speakerLine}\n${bodyLine}`;

    console.log(formatted);
    console.log();
  }

  renderFullMessage(msg: FormattedMessage): void {
    const color = msg.type === 'user'
      ? USER_COLOR
      : getAgentColor(msg.sender, msg.type, this.colorMap);

    const agentBases = getAgentBases(this.db);

    // No truncation - show full body
    const colorizedBody = color
      ? highlightIssueIds(colorizeBody(msg.body, color, agentBases, this.colorMap), color)
      : highlightIssueIds(unescapeBody(msg.body));

    const speakerContent = ` @${msg.sender}: `;

    let speakerLine: string;
    if (color) {
      const textColor = getContrastTextColor(color);
      const bgColor = color.replace('38;5;', '48;5;');
      speakerLine = `${bgColor}${BOLD}${textColor}${speakerContent}${RESET}`;
    } else {
      speakerLine = `${BOLD}${speakerContent}${RESET}`;
    }

    const idSuffix = `${DIM} #${extractGuidPrefix(msg.id)}${RESET}`;
    const bodyLine = color ? `${color}${colorizedBody}${RESET}${idSuffix}` : `${colorizedBody}${idSuffix}`;
    const replyContext = msg.reply_to ? this.buildReplyContext(msg.reply_to) : '';
    const formatted = replyContext
      ? `${speakerLine}\n${replyContext}\n${bodyLine}`
      : `${speakerLine}\n${bodyLine}`;

    console.log(formatted);
    console.log();
  }

  showStatus(text: string): void {
    console.log(`${DIM}${text}${RESET}`);
  }

  destroy(): void {
  }

  private getPrefixLength(): number {
    const row = this.db.prepare(`
      SELECT COUNT(*) as count
      FROM mm_messages
      WHERE archived_at IS NULL
    `).get() as { count: number } | undefined;
    return getDisplayPrefixLength(row?.count ?? 0);
  }

  private buildReplyContext(replyTo: string, prefixLength: number): string {
    const row = this.db.prepare(`
      SELECT from_agent, body
      FROM mm_messages
      WHERE guid = ?
    `).get(replyTo) as { from_agent: string; body: string } | undefined;

    if (!row) {
      const prefix = getGuidPrefix(replyTo, prefixLength);
      return `${DIM} ${BOLD}↪${RESET}${DIM}${ITALIC} Reply to #${prefix}${RESET}`;
    }

    const preview = truncatePreview(row.body);
    return `${DIM} ${BOLD}↪${RESET}${DIM}${ITALIC} Reply to @${row.from_agent}: "${preview}"${RESET}`;
  }
}
