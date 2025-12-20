import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { formatRelative, isStale, now } from '../src/core/time.ts';

describe('now', () => {
  it('should return unix timestamp in seconds', () => {
    const timestamp = now();
    const expected = Math.floor(Date.now() / 1000);

    // Allow 1 second tolerance
    expect(Math.abs(timestamp - expected)).toBeLessThanOrEqual(1);
  });

  it('should return integer', () => {
    const timestamp = now();
    expect(Number.isInteger(timestamp)).toBe(true);
  });
});

describe('formatRelative', () => {
  beforeEach(() => {
    // Mock Date.now to return a fixed timestamp
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-01T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('should format seconds correctly', () => {
    const current = now();
    expect(formatRelative(current - 0)).toBe('0s ago');
    expect(formatRelative(current - 1)).toBe('1s ago');
    expect(formatRelative(current - 30)).toBe('30s ago');
    expect(formatRelative(current - 59)).toBe('59s ago');
  });

  it('should format minutes correctly', () => {
    const current = now();
    expect(formatRelative(current - 60)).toBe('1m ago');
    expect(formatRelative(current - 120)).toBe('2m ago');
    expect(formatRelative(current - 300)).toBe('5m ago');
    expect(formatRelative(current - 3540)).toBe('59m ago');
  });

  it('should format hours correctly', () => {
    const current = now();
    expect(formatRelative(current - 3600)).toBe('1h ago');
    expect(formatRelative(current - 7200)).toBe('2h ago');
    expect(formatRelative(current - 86340)).toBe('23h ago');
  });

  it('should format days correctly', () => {
    const current = now();
    expect(formatRelative(current - 86400)).toBe('1d ago');
    expect(formatRelative(current - 172800)).toBe('2d ago');
    expect(formatRelative(current - 518400)).toBe('6d ago');
  });

  it('should format weeks correctly', () => {
    const current = now();
    expect(formatRelative(current - 604800)).toBe('1w ago');
    expect(formatRelative(current - 1209600)).toBe('2w ago');
    expect(formatRelative(current - 2592000)).toBe('4w ago');
  });

  it('should handle boundary values', () => {
    const current = now();
    // 59 seconds should be seconds
    expect(formatRelative(current - 59)).toBe('59s ago');
    // 60 seconds should be 1 minute
    expect(formatRelative(current - 60)).toBe('1m ago');

    // 59 minutes should be minutes
    expect(formatRelative(current - 3540)).toBe('59m ago');
    // 60 minutes should be 1 hour
    expect(formatRelative(current - 3600)).toBe('1h ago');

    // 23 hours should be hours
    expect(formatRelative(current - 82800)).toBe('23h ago');
    // 24 hours should be 1 day
    expect(formatRelative(current - 86400)).toBe('1d ago');

    // 6 days should be days
    expect(formatRelative(current - 518400)).toBe('6d ago');
    // 7 days should be 1 week
    expect(formatRelative(current - 604800)).toBe('1w ago');
  });

  it('should handle future timestamps', () => {
    const current = now();
    expect(formatRelative(current + 100)).toBe('just now');
  });

  it('should use floor division', () => {
    const current = now();
    // 90 seconds = 1.5 minutes, should floor to 1 minute
    expect(formatRelative(current - 90)).toBe('1m ago');
    // 5400 seconds = 1.5 hours, should floor to 1 hour
    expect(formatRelative(current - 5400)).toBe('1h ago');
    // 129600 seconds = 1.5 days, should floor to 1 day
    expect(formatRelative(current - 129600)).toBe('1d ago');
  });
});

describe('isStale', () => {
  beforeEach(() => {
    // Mock Date.now to return a fixed timestamp
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-01T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('should detect stale agents', () => {
    const current = now();

    // 5 hours ago with 4 hour threshold = stale
    expect(isStale(current - 5 * 3600, 4)).toBe(true);

    // 10 hours ago with 4 hour threshold = stale
    expect(isStale(current - 10 * 3600, 4)).toBe(true);
  });

  it('should detect active agents', () => {
    const current = now();

    // 3 hours ago with 4 hour threshold = not stale
    expect(isStale(current - 3 * 3600, 4)).toBe(false);

    // 1 hour ago with 4 hour threshold = not stale
    expect(isStale(current - 1 * 3600, 4)).toBe(false);

    // Just now with 4 hour threshold = not stale
    expect(isStale(current, 4)).toBe(false);
  });

  it('should handle boundary cases', () => {
    const current = now();

    // Exactly at threshold should be active (not stale)
    expect(isStale(current - 4 * 3600, 4)).toBe(false);

    // 1 second past threshold should be stale
    expect(isStale(current - 4 * 3600 - 1, 4)).toBe(true);
  });

  it('should work with different threshold values', () => {
    const current = now();

    // 25 hours ago with 24 hour threshold = stale
    expect(isStale(current - 25 * 3600, 24)).toBe(true);

    // 23 hours ago with 24 hour threshold = not stale
    expect(isStale(current - 23 * 3600, 24)).toBe(false);

    // 2 hours ago with 1 hour threshold = stale
    expect(isStale(current - 2 * 3600, 1)).toBe(true);

    // 30 minutes ago with 1 hour threshold = not stale
    expect(isStale(current - 1800, 1)).toBe(false);
  });

  it('should handle future timestamps', () => {
    const current = now();

    // Future timestamp should not be stale
    expect(isStale(current + 1000, 4)).toBe(false);
  });
});
