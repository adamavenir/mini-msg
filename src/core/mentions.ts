import { matchesPrefix } from './agents.js';
import type Database from 'better-sqlite3';
import { getAgentBases } from '../db/queries.js';

/**
 * Regex pattern for matching @mentions.
 * Matches @ followed by:
 * - "all" (broadcast)
 * - OR valid agent ID/prefix: starts with letter, then letters/numbers/dots/hyphens
 * Uses negative lookbehind (?<![a-zA-Z0-9]) to avoid matching email addresses
 */
const MENTION_REGEX = /(?<![a-zA-Z0-9])@(all|[a-z][a-z0-9]*(?:[-\.][a-z0-9]+)*)/g;

/**
 * Regex pattern for matching issue references.
 * Matches @prefix-id where:
 * - prefix: lowercase letters (project prefix)
 * - id: alphanumeric characters (case-insensitive)
 * Uses negative lookbehind (?<![a-zA-Z0-9]) to avoid matching within words
 */
const ISSUE_REF_REGEX = /(?<![a-zA-Z0-9])@([a-z]+-[a-zA-Z0-9]+)/g;

/**
 * Extract all @mentions from message body.
 * Returns mention targets without @ prefix.
 * When db is provided, validates plain names against known agent bases.
 * @param body - Message text to extract mentions from
 * @param db - Optional database connection for validation
 * @example extractMentions("hey @alice.1 and @bob.3") -> ["alice.1", "bob.3"]
 * @example extractMentions("@all heads up") -> ["all"]
 * @example extractMentions("@alice check this", db) -> ["alice"] (if alice.N exists)
 * @example extractMentions("@state and @payload", db) -> [] (code variables filtered out)
 */
export function extractMentions(body: string, db?: Database.Database): string[] {
  const mentions: string[] = [];
  const matches = body.matchAll(MENTION_REGEX);

  // Get known agent bases if db provided
  const agentBases = db ? getAgentBases(db) : null;

  for (const match of matches) {
    const name = match[1];

    // Always include @all
    if (name === 'all') {
      mentions.push(name);
      continue;
    }

    // Include if it has a dot (agent ID like alice.1)
    if (name.includes('.')) {
      mentions.push(name);
      continue;
    }

    // For plain names (e.g., alice), check against agent bases
    if (agentBases) {
      if (agentBases.has(name)) {
        mentions.push(name);
      }
      // else skip - not a known agent base
    } else {
      // No DB provided - use fallback heuristic
      // Include names that are 3-15 characters (typical usernames)
      if (name.length >= 3 && name.length <= 15) {
        mentions.push(name);
      }
    }
  }

  return mentions;
}

/**
 * Extract issue references from text.
 * Matches @prefix-id pattern where prefix is letters and id is alphanumeric.
 * @returns Array of issue IDs without @ prefix, deduplicated
 * @example extractIssueRefs("@mm-123 hello") -> ["mm-123"]
 * @example extractIssueRefs("@mm-123 and @api-456") -> ["mm-123", "api-456"]
 * @example extractIssueRefs("@alice.1 @mm-123") -> ["mm-123"]
 */
export function extractIssueRefs(body: string): string[] {
  const refs = new Set<string>();
  const matches = body.matchAll(ISSUE_REF_REGEX);

  for (const match of matches) {
    refs.add(match[1].toLowerCase());
  }

  return Array.from(refs);
}

/**
 * Check if an agent ID matches a mention prefix.
 * Used to determine if a message is relevant to an agent.
 * @example matchesMention("alice.419", "alice") -> true (prefix match)
 * @example matchesMention("alice.419", "alice.419") -> true (exact match)
 * @example matchesMention("alice.frontend.3", "alice") -> true (prefix match)
 * @example matchesMention("bob.1", "alice") -> false
 */
export function matchesMention(agentId: string, mentionPrefix: string): boolean {
  return matchesPrefix(agentId, mentionPrefix);
}

/**
 * Check if a mention is the broadcast mention.
 */
export function isAllMention(mention: string): boolean {
  return mention === 'all';
}

/**
 * Highlight mentions in message body for display.
 * Respects NO_COLOR environment variable.
 * @example highlightMentions("hey @alice") -> "hey \x1b[36m@alice\x1b[0m"
 */
export function highlightMentions(body: string): string {
  if (process.env.NO_COLOR) {
    return body;
  }
  return body.replace(MENTION_REGEX, '\x1b[36m$&\x1b[0m');
}

/**
 * Normalize agent reference (strip leading @).
 * @example normalizeAgentRef("@alice.1") -> "alice.1"
 * @example normalizeAgentRef("alice.1") -> "alice.1"
 */
export function normalizeAgentRef(ref: string): string {
  return ref.startsWith('@') ? ref.substring(1) : ref;
}
