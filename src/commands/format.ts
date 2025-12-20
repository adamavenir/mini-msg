import path from 'path';
import { highlightMentions } from '../core/mentions.js';
import { parseAgentId } from '../core/agents.js';

const NO_COLOR = !!process.env.NO_COLOR;

const DIM = NO_COLOR ? '' : '\x1b[2m';
const BOLD = NO_COLOR ? '' : '\x1b[1m';
const BLUE = NO_COLOR ? '' : '\x1b[34m';
const GRAY = NO_COLOR ? '' : '\x1b[38;5;240m';
const RESET = NO_COLOR ? '' : '\x1b[0m';
const ISSUE_BG = NO_COLOR ? '' : '\x1b[48;5;17m';
const WHITE_TEXT = NO_COLOR ? '' : '\x1b[38;5;231m';

const MAX_DISPLAY_LINES = 20;

const USER_COLOR = NO_COLOR ? '' : '\x1b[38;5;249m';

export const COLOR_PAIRS = [
  { bright: '\x1b[38;5;111m', dim: '\x1b[38;5;105m' },  // Blue
  { bright: '\x1b[38;5;157m', dim: '\x1b[38;5;156m' },  // Green
  { bright: '\x1b[38;5;216m', dim: '\x1b[38;5;215m' }, // Orange
  { bright: '\x1b[38;5;36m', dim: '\x1b[38;5;30m' },   // Teal
  { bright: '\x1b[38;5;183m', dim: '\x1b[38;5;141m' },  // Purple
  { bright: '\x1b[38;5;230m', dim: '\x1b[38;5;229m' }, // Yellow
];

/**
 * Simple hash function for strings.
 * Returns a non-negative integer.
 */
function hashString(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash; // Convert to 32-bit integer
  }
  return Math.abs(hash);
}

/**
 * Get ANSI color code for an agent name with optional color map.
 * - Users (type='user'): no color
 * - System messages (from_agent='system'): gray
 * - Agents: color based on map (if provided), or hash of base name
 * - Bright/dim based on version parity
 * @param agentId Agent ID (e.g., "alice.1")
 * @param type Message type ('agent' or 'user')
 * @param colorMap Optional map from agent base to color index (0-5)
 * @returns ANSI color code string
 */
export function getAgentColor(agentId: string, type: string = 'agent', colorMap?: Map<string, number>): string {
  if (NO_COLOR) {
    return '';
  }

  if (agentId === 'system') {
    return GRAY;
  }

  try {
    const parsed = parseAgentId(agentId);

    // Use color map if provided and base exists in map
    let colorIndex: number;
    if (colorMap && colorMap.has(parsed.base)) {
      colorIndex = colorMap.get(parsed.base)!;
    } else {
      // Fall back to hash-based assignment
      colorIndex = hashString(parsed.base) % COLOR_PAIRS.length;
    }

    const pair = COLOR_PAIRS[colorIndex];

    // Odd version = bright, even version = dim
    return parsed.version % 2 === 1 ? pair.bright : pair.dim;
  } catch {
    // Plain name without version (e.g., "adam") - treat as version 1
    const colorIndex = hashString(agentId) % COLOR_PAIRS.length;
    const pair = COLOR_PAIRS[colorIndex];
    return pair.bright;
  }
}

/**
 * Extract project name from project path.
 * Uses the directory name containing .mm/
 */
export function getProjectName(project: { root: string; dbPath: string }): string {
  return path.basename(project.root);
}

/**
 * Highlight issue IDs with dark blue background and white text.
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
 * Replaces @mentions with colored versions matching the mentioned agent.
 * Validates against known agent bases when provided.
 * @param body Message body text
 * @param senderColor The sender's color code (used to reset after each mention)
 * @param agentBases Optional set of known agent base names for validation
 * @returns Body with colorized mentions
 */
function colorizeBody(body: string, senderColor: string, agentBases?: Set<string>): string {
  return body.replace(/@([a-z][a-z0-9.]*)/gi, (match, name) => {
    const lowercase = name.toLowerCase();

    // Always colorize @all
    if (lowercase === 'all') {
      const mentionColor = getAgentColor(name, 'agent');
      if (mentionColor) {
        return `${mentionColor}${match}${senderColor}`;
      }
    }

    // Colorize if it has a dot (agent ID like alice.1)
    if (name.includes('.')) {
      const mentionColor = getAgentColor(name, 'agent');
      if (mentionColor) {
        return `${mentionColor}${match}${senderColor}`;
      }
    }

    // For plain names, validate against agent bases if provided
    if (agentBases) {
      if (agentBases.has(lowercase)) {
        const mentionColor = getAgentColor(name, 'agent');
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
        const mentionColor = getAgentColor(name, 'agent');
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
         `... (${remaining} more lines. Use 'mm view ${msgId}' to see full)`;
}

/**
 * Format a message for display.
 * Format: [#projectname guid] @speaker: "message"
 * - Project/message GUID block is dimmed
 * - Project name (#projectname) is bold within the dim block
 * - Entire message body is colorized based on sender agent
 * - @mentions within body are colorized to match mentioned agent
 */
export function formatMessage(msg: any, projectName: string, agentBases?: Set<string>): string {
  const idBlock = `${DIM}[${BOLD}#${projectName}${RESET}${DIM} ${msg.id}]${RESET}`;

  const color = getAgentColor(msg.from_agent, msg.type);
  const truncatedBody = truncateForDisplay(msg.body, msg.id);

  if (color) {
    // Color entire message body including speaker and text
    // Apply @mention colorization first, then issue ID highlighting
    const colorizedBody = highlightIssueIds(colorizeBody(truncatedBody, color, agentBases), color);
    return `${idBlock} ${color}@${msg.from_agent}: "${colorizedBody}"${RESET}`;
  } else {
    // User messages - no color
    const body = highlightIssueIds(highlightMentions(truncatedBody));
    return `${idBlock} @${msg.from_agent}: "${body}"`;
  }
}
