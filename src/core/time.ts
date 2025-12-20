/**
 * Get current unix timestamp.
 */
export function now(): number {
  return Math.floor(Date.now() / 1000);
}

/**
 * Format unix timestamp as relative time.
 * @example formatRelative(now - 120) -> "2m ago"
 * @example formatRelative(now - 3600) -> "1h ago"
 * @example formatRelative(now - 86400) -> "1d ago"
 */
export function formatRelative(ts: number): string {
  const secondsAgo = now() - ts;

  // Future timestamps
  if (secondsAgo < 0) {
    return 'just now';
  }

  // Seconds
  if (secondsAgo < 60) {
    return `${secondsAgo}s ago`;
  }

  // Minutes
  const minutesAgo = Math.floor(secondsAgo / 60);
  if (minutesAgo < 60) {
    return `${minutesAgo}m ago`;
  }

  // Hours
  const hoursAgo = Math.floor(secondsAgo / 3600);
  if (hoursAgo < 24) {
    return `${hoursAgo}h ago`;
  }

  // Days
  const daysAgo = Math.floor(secondsAgo / 86400);
  if (daysAgo < 7) {
    return `${daysAgo}d ago`;
  }

  // Weeks
  const weeksAgo = Math.floor(secondsAgo / (86400 * 7));
  return `${weeksAgo}w ago`;
}

/**
 * Check if an agent is stale based on last seen time.
 * @param lastSeen - unix timestamp of last activity
 * @param staleHours - hours of inactivity threshold
 */
export function isStale(lastSeen: number, staleHours: number): boolean {
  return lastSeen + (staleHours * 3600) < now();
}
