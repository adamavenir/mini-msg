import { describe, it, expect } from 'vitest';
import { generateGuid } from '../src/core/guid.ts';

// Lowercase alphanumeric (36 chars)
const ALPHANUM_RE = /^[0-9a-z]{8}$/;

describe('generateGuid', () => {
  it('should generate GUIDs with prefix and alphanumeric body', () => {
    const guid = generateGuid('msg');
    expect(guid.startsWith('msg-')).toBe(true);
    const body = guid.split('-')[1];
    expect(body).toBeDefined();
    expect(body).toMatch(ALPHANUM_RE);
  });

  it('should normalize trailing dash in prefix', () => {
    const guid = generateGuid('usr-');
    expect(guid.startsWith('usr-')).toBe(true);
    const body = guid.split('-')[1];
    expect(body).toMatch(ALPHANUM_RE);
  });

  it('should generate unique GUIDs across multiple calls', () => {
    const seen = new Set<string>();
    for (let i = 0; i < 100; i += 1) {
      const guid = generateGuid('msg');
      expect(seen.has(guid)).toBe(false);
      seen.add(guid);
    }
  });

  it('should support different prefixes', () => {
    const msg = generateGuid('msg');
    const usr = generateGuid('usr');
    const ch = generateGuid('ch');

    expect(msg).toMatch(/^msg-[0-9a-z]{8}$/);
    expect(usr).toMatch(/^usr-[0-9a-z]{8}$/);
    expect(ch).toMatch(/^ch-[0-9a-z]{8}$/);
  });
});
