import crypto from 'crypto';

// Lowercase letters and numbers only (36 chars) - easier to type than mixed case
const ALPHABET = '0123456789abcdefghijklmnopqrstuvwxyz';
const GUID_LENGTH = 8;
const DISPLAY_LENGTH_SMALL = 4;
const DISPLAY_LENGTH_MEDIUM = 5;
const DISPLAY_LENGTH_LARGE = 6;

/**
 * Generate a short GUID with a prefix (e.g., msg-xxxx).
 * Uses lowercase alphanumeric characters for easy typing.
 */
export function generateGuid(prefix: string): string {
  const normalizedPrefix = prefix.endsWith('-') ? prefix.slice(0, -1) : prefix;
  const bytes = crypto.randomBytes(GUID_LENGTH);
  let id = '';

  for (let i = 0; i < GUID_LENGTH; i++) {
    id += ALPHABET[bytes[i] % ALPHABET.length];
  }

  return `${normalizedPrefix}-${id}`;
}

export function getDisplayPrefixLength(messageCount: number): number {
  if (messageCount < 500) {
    return DISPLAY_LENGTH_SMALL;
  }
  if (messageCount < 1500) {
    return DISPLAY_LENGTH_MEDIUM;
  }
  return DISPLAY_LENGTH_LARGE;
}

export function getGuidPrefix(guid: string, length: number): string {
  const base = guid.startsWith('msg-') ? guid.slice(4) : guid;
  if (length <= 0) return '';
  return base.slice(0, Math.min(length, base.length));
}
