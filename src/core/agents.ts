import type { ParsedAgentId } from '../types.js';

/**
 * Check if an agent ID is in legacy format (has version suffix like alice.1).
 */
export function isLegacyAgentId(id: string): boolean {
  if (typeof id !== 'string' || id.length === 0) {
    return false;
  }
  const lastDotIndex = id.lastIndexOf('.');
  if (lastDotIndex === -1) {
    return false;
  }
  const versionStr = id.substring(lastDotIndex + 1);
  const version = parseInt(versionStr, 10);
  return Number.isInteger(version) && version > 0 && version.toString() === versionStr;
}

/**
 * Parse an agent ID into components.
 * Supports both legacy (alice.1) and simple (alice, eager-beaver) formats.
 * @throws if ID is invalid
 * @example parseAgentId("alice.419") -> { base: "alice", version: 419, full: "alice.419" }
 * @example parseAgentId("alice") -> { base: "alice", version: undefined, full: "alice" }
 * @example parseAgentId("eager-beaver") -> { base: "eager-beaver", version: undefined, full: "eager-beaver" }
 */
export function parseAgentId(id: string): ParsedAgentId {
  if (!isValidAgentId(id)) {
    throw new Error(`Invalid agent ID: ${id}`);
  }

  // Check if it's a legacy format with version
  if (isLegacyAgentId(id)) {
    const lastDotIndex = id.lastIndexOf('.');
    const base = id.substring(0, lastDotIndex);
    const version = parseInt(id.substring(lastDotIndex + 1), 10);
    return {
      base,
      version,
      full: id,
    };
  }

  // Simple format - no version
  return {
    base: id,
    version: undefined,
    full: id,
  };
}

/**
 * Format base and version into agent ID.
 * @deprecated Use simple agent names instead of versioned IDs
 * @example formatAgentId("alice", 419) -> "alice.419"
 */
export function formatAgentId(base: string, version: number): string {
  if (!isValidBaseName(base)) {
    throw new Error(`Invalid base name: ${base}`);
  }
  if (!Number.isInteger(version) || version <= 0) {
    throw new Error(`Invalid version: ${version}`);
  }
  return `${base}.${version}`;
}

/**
 * Validate agent ID format.
 * Accepts simple names (alice, eager-beaver) and dotted names (alice.frontend, alice.1).
 * @returns true if valid, false otherwise
 */
export function isValidAgentId(id: string): boolean {
  if (typeof id !== 'string' || id.length === 0) {
    return false;
  }

  // Check for invalid patterns
  if (id.includes('..') || id.startsWith('.') || id.startsWith('-') || id.endsWith('.') || id.endsWith('-')) {
    return false;
  }

  // Simple name format: lowercase letters, numbers, hyphens
  // Must start with a letter, can contain hyphens for generated names like "eager-beaver"
  const simpleNameRegex = /^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)*$/;
  if (simpleNameRegex.test(id)) {
    return true;
  }

  // Dotted format: validate as base name (allows alice.frontend, alice.1, pm.3.sub)
  return isValidBaseName(id);
}

/**
 * Validate base name format (role or role.qualifier).
 * @returns true if valid base name
 */
export function isValidBaseName(base: string): boolean {
  if (typeof base !== 'string' || base.length === 0) {
    return false;
  }

  // Check for empty segments
  if (base.includes('..') || base.startsWith('.') || base.endsWith('.')) {
    return false;
  }

  // Simple name with hyphens (for generated names like "eager-beaver")
  const simpleNameRegex = /^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)*$/;
  if (simpleNameRegex.test(base)) {
    return true;
  }

  // Split into segments for dotted names
  const segments = base.split('.');

  // First segment must start with a letter (not a pure number)
  const simpleSegmentRegex = /^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)*$/;
  if (!simpleSegmentRegex.test(segments[0])) {
    return false;
  }

  // Subsequent segments can be simple names or positive integers
  const positiveIntRegex = /^[1-9][0-9]*$/;

  for (let i = 1; i < segments.length; i++) {
    const segment = segments[i];
    if (!simpleSegmentRegex.test(segment) && !positiveIntRegex.test(segment)) {
      return false;
    }
  }

  return true;
}

/**
 * Alias for isValidBaseName.
 * Validates agent name format.
 */
export function isValidAgentBase(base: string): boolean {
  return isValidBaseName(base);
}

/**
 * Normalize agent reference (strip leading @).
 * @example normalizeAgentRef("@alice.1") -> "alice.1"
 * @example normalizeAgentRef("alice.1") -> "alice.1"
 */
export function normalizeAgentRef(ref: string): string {
  return ref.startsWith('@') ? ref.substring(1) : ref;
}

/**
 * Check if an agent ID matches a prefix.
 * Used for @mention resolution.
 * @example matchesPrefix("alice.419", "alice") -> true
 * @example matchesPrefix("alice.frontend.3", "alice") -> true
 * @example matchesPrefix("bob.1", "alice") -> false
 */
export function matchesPrefix(agentId: string, prefix: string): boolean {
  const normalizedPrefix = normalizeAgentRef(prefix);

  // Exact match
  if (agentId === normalizedPrefix) {
    return true;
  }

  // Prefix match: agentId must start with prefix followed by a dot
  if (agentId.startsWith(normalizedPrefix + '.')) {
    return true;
  }

  return false;
}
