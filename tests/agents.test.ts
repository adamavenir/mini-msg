import { describe, it, expect } from 'vitest';
import {
  parseAgentId,
  formatAgentId,
  isValidAgentId,
  isValidBaseName,
  normalizeAgentRef,
  matchesPrefix,
} from '../src/core/agents.ts';

describe('parseAgentId', () => {
  it('should parse simple agent ID', () => {
    const result = parseAgentId('alice.1');
    expect(result).toEqual({
      base: 'alice',
      version: 1,
      full: 'alice.1',
    });
  });

  it('should parse agent ID with qualifier', () => {
    const result = parseAgentId('alice.frontend.3');
    expect(result).toEqual({
      base: 'alice.frontend',
      version: 3,
      full: 'alice.frontend.3',
    });
  });

  it('should parse complex agent ID', () => {
    const result = parseAgentId('pm.5.sub.2');
    expect(result).toEqual({
      base: 'pm.5.sub',
      version: 2,
      full: 'pm.5.sub.2',
    });
  });

  it('should parse agent ID with large version', () => {
    const result = parseAgentId('alice.419');
    expect(result).toEqual({
      base: 'alice',
      version: 419,
      full: 'alice.419',
    });
  });

  it('should throw on invalid agent ID', () => {
    // Simple names are now valid
    expect(() => parseAgentId('alice.')).toThrow('Invalid agent ID');
    expect(() => parseAgentId('.alice.1')).toThrow('Invalid agent ID');
    expect(() => parseAgentId('alice..1')).toThrow('Invalid agent ID');
    expect(() => parseAgentId('alice.0')).toThrow('Invalid agent ID');
    expect(() => parseAgentId('alice.-1')).toThrow('Invalid agent ID');
    expect(() => parseAgentId('-alice')).toThrow('Invalid agent ID');
    expect(() => parseAgentId('alice-')).toThrow('Invalid agent ID');
  });

  it('should parse dotted names without version', () => {
    const result = parseAgentId('alice.frontend');
    expect(result.base).toBe('alice.frontend');
    expect(result.version).toBeUndefined();
    expect(result.full).toBe('alice.frontend');
  });

  it('should parse simple names (no version)', () => {
    const result = parseAgentId('alice');
    expect(result).toEqual({
      base: 'alice',
      version: undefined,
      full: 'alice',
    });
  });

  it('should parse hyphenated names', () => {
    const result = parseAgentId('eager-beaver');
    expect(result).toEqual({
      base: 'eager-beaver',
      version: undefined,
      full: 'eager-beaver',
    });
  });
});

describe('formatAgentId', () => {
  it('should format simple agent ID', () => {
    expect(formatAgentId('alice', 1)).toBe('alice.1');
  });

  it('should format agent ID with qualifier', () => {
    expect(formatAgentId('alice.frontend', 3)).toBe('alice.frontend.3');
  });

  it('should format complex agent ID', () => {
    expect(formatAgentId('pm.5.sub', 2)).toBe('pm.5.sub.2');
  });

  it('should throw on invalid base', () => {
    expect(() => formatAgentId('', 1)).toThrow('Invalid base name');
    expect(() => formatAgentId('Alice', 1)).toThrow('Invalid base name');
    expect(() => formatAgentId('alice.', 1)).toThrow('Invalid base name');
    expect(() => formatAgentId('.alice', 1)).toThrow('Invalid base name');
    expect(() => formatAgentId('alice..frontend', 1)).toThrow('Invalid base name');
  });

  it('should throw on invalid version', () => {
    expect(() => formatAgentId('alice', 0)).toThrow('Invalid version');
    expect(() => formatAgentId('alice', -1)).toThrow('Invalid version');
    expect(() => formatAgentId('alice', 1.5)).toThrow('Invalid version');
  });
});

describe('isValidAgentId', () => {
  it('should accept valid simple IDs', () => {
    expect(isValidAgentId('alice.1')).toBe(true);
    expect(isValidAgentId('bob.2')).toBe(true);
    expect(isValidAgentId('pm.419')).toBe(true);
  });

  it('should accept valid IDs with qualifiers', () => {
    expect(isValidAgentId('alice.frontend.3')).toBe(true);
    expect(isValidAgentId('pm.5.sub.2')).toBe(true);
    expect(isValidAgentId('agent.a.b.c.1')).toBe(true);
  });

  it('should accept IDs with alphanumeric segments', () => {
    expect(isValidAgentId('alice2.1')).toBe(true);
    expect(isValidAgentId('agent42.sub3.1')).toBe(true);
  });

  it('should accept simple names without dots', () => {
    expect(isValidAgentId('alice')).toBe(true);
    expect(isValidAgentId('pm')).toBe(true);
    expect(isValidAgentId('eager-beaver')).toBe(true);
    expect(isValidAgentId('cosmic-dolphin')).toBe(true);
  });

  it('should reject invalid simple names', () => {
    expect(isValidAgentId('123')).toBe(false);
    expect(isValidAgentId('-alice')).toBe(false);
    expect(isValidAgentId('alice-')).toBe(false);
    expect(isValidAgentId('Alice')).toBe(false);
  });

  it('should reject IDs with invalid segments', () => {
    expect(isValidAgentId('alice.0')).toBe(false);    // Zero not allowed
    expect(isValidAgentId('alice.-1')).toBe(false);   // Negative not allowed
    expect(isValidAgentId('alice.01')).toBe(false);   // Leading zero not allowed
  });

  it('should accept dotted names without version', () => {
    expect(isValidAgentId('alice.frontend')).toBe(true);
    expect(isValidAgentId('alice.frontend.1')).toBe(true);
    expect(isValidAgentId('my-agent.sub-task')).toBe(true);
    expect(isValidAgentId('pm.3.sub')).toBe(true);
  });

  it('should reject IDs with empty segments', () => {
    expect(isValidAgentId('.alice.1')).toBe(false);
    expect(isValidAgentId('alice..1')).toBe(false);
    expect(isValidAgentId('alice.1.')).toBe(false);
  });

  it('should reject IDs with uppercase letters', () => {
    expect(isValidAgentId('Alice.1')).toBe(false);
    expect(isValidAgentId('alice.Frontend.1')).toBe(false);
  });

  it('should reject IDs with leading digits in segments', () => {
    expect(isValidAgentId('1alice.1')).toBe(false);
    expect(isValidAgentId('alice.2frontend.1')).toBe(false);
  });

  it('should reject invalid types', () => {
    expect(isValidAgentId('')).toBe(false);
  });
});

describe('isValidBaseName', () => {
  it('should accept valid simple base names', () => {
    expect(isValidBaseName('alice')).toBe(true);
    expect(isValidBaseName('bob')).toBe(true);
    expect(isValidBaseName('pm')).toBe(true);
  });

  it('should accept valid base names with qualifiers', () => {
    expect(isValidBaseName('alice.frontend')).toBe(true);
    expect(isValidBaseName('pm.5.sub')).toBe(true);
    expect(isValidBaseName('agent.a.b.c')).toBe(true);
  });

  it('should accept base names with alphanumeric segments', () => {
    expect(isValidBaseName('alice2')).toBe(true);
    expect(isValidBaseName('agent42.sub3')).toBe(true);
  });

  it('should reject base names with uppercase', () => {
    expect(isValidBaseName('Alice')).toBe(false);
    expect(isValidBaseName('alice.Frontend')).toBe(false);
  });

  it('should reject base names with leading digits', () => {
    expect(isValidBaseName('1alice')).toBe(false);
    expect(isValidBaseName('alice.2frontend')).toBe(false);
  });

  it('should reject base names with empty segments', () => {
    expect(isValidBaseName('.alice')).toBe(false);
    expect(isValidBaseName('alice.')).toBe(false);
    expect(isValidBaseName('alice..frontend')).toBe(false);
  });

  it('should reject empty base names', () => {
    expect(isValidBaseName('')).toBe(false);
  });
});

describe('normalizeAgentRef', () => {
  it('should strip leading @', () => {
    expect(normalizeAgentRef('@alice.1')).toBe('alice.1');
    expect(normalizeAgentRef('@alice')).toBe('alice');
    expect(normalizeAgentRef('@all')).toBe('all');
  });

  it('should not modify refs without @', () => {
    expect(normalizeAgentRef('alice.1')).toBe('alice.1');
    expect(normalizeAgentRef('alice')).toBe('alice');
    expect(normalizeAgentRef('all')).toBe('all');
  });

  it('should only strip leading @', () => {
    expect(normalizeAgentRef('alice@1')).toBe('alice@1');
    expect(normalizeAgentRef('alice.1@')).toBe('alice.1@');
  });
});

describe('matchesPrefix', () => {
  it('should match exact agent IDs', () => {
    expect(matchesPrefix('alice.1', 'alice.1')).toBe(true);
    expect(matchesPrefix('alice.419', 'alice.419')).toBe(true);
  });

  it('should match base prefixes', () => {
    expect(matchesPrefix('alice.1', 'alice')).toBe(true);
    expect(matchesPrefix('alice.419', 'alice')).toBe(true);
    expect(matchesPrefix('alice.frontend.3', 'alice')).toBe(true);
  });

  it('should match qualified prefixes', () => {
    expect(matchesPrefix('alice.frontend.3', 'alice.frontend')).toBe(true);
    expect(matchesPrefix('pm.5.sub.2', 'pm.5')).toBe(true);
    expect(matchesPrefix('pm.5.sub.2', 'pm.5.sub')).toBe(true);
  });

  it('should not match different bases', () => {
    expect(matchesPrefix('bob.1', 'alice')).toBe(false);
    expect(matchesPrefix('alice.frontend.1', 'bob')).toBe(false);
  });

  it('should not match partial segment names', () => {
    expect(matchesPrefix('alice.1', 'ali')).toBe(false);
    expect(matchesPrefix('alice.frontend.1', 'alice.front')).toBe(false);
  });

  it('should handle @ prefix in mention', () => {
    expect(matchesPrefix('alice.1', '@alice')).toBe(true);
    expect(matchesPrefix('alice.419', '@alice')).toBe(true);
    expect(matchesPrefix('bob.1', '@alice')).toBe(false);
  });
});
