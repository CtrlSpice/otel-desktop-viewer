const RECENT_STORAGE_KEY = 'datetime-filter-recent';
const MAX_RECENT = 10;

export type RecentTimeRange = {
  start: number;
  end: number;
  usedAt: number;
};

export function loadRecentTimeRanges(): RecentTimeRange[] {
  try {
    const saved = localStorage.getItem(RECENT_STORAGE_KEY);
    if (!saved) return [];
    const parsed: unknown = JSON.parse(saved);
    if (!Array.isArray(parsed)) return [];
    return parsed as RecentTimeRange[];
  } catch {
    return [];
  }
}

/** Add or bump a range in recents (dedupe by start/end). Persists to localStorage. */
export function recordRecentTimeRange(
  start: number,
  end: number,
  usedAt: number
): void {
  let recentTimeRanges = loadRecentTimeRanges();
  const existingIndex = recentTimeRanges.findIndex(
    e => e.start === start && e.end === end
  );

  if (existingIndex !== -1) {
    const updated = [...recentTimeRanges];
    updated[existingIndex] = { ...updated[existingIndex], usedAt };
    recentTimeRanges = updated.sort((a, b) => b.usedAt - a.usedAt);
  } else {
    recentTimeRanges = [{ start, end, usedAt }, ...recentTimeRanges]
      .sort((a, b) => b.usedAt - a.usedAt)
      .slice(0, MAX_RECENT);
  }

  localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(recentTimeRanges));
}
