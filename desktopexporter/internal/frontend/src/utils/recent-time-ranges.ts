const RECENT_STORAGE_KEY = 'datetime-filter-recent';

/** Stored in localStorage and shown in the datetime popover — keep in sync. */
export const MAX_RECENT_TIME_RANGES = 5;

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
    const rows = parsed as RecentTimeRange[];
    const sorted = [...rows].sort((a, b) => b.usedAt - a.usedAt);
    const trimmed = sorted.slice(0, MAX_RECENT_TIME_RANGES);
    if (trimmed.length < rows.length) {
      localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(trimmed));
    }
    return trimmed;
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
    recentTimeRanges = updated
      .sort((a, b) => b.usedAt - a.usedAt)
      .slice(0, MAX_RECENT_TIME_RANGES);
  } else {
    recentTimeRanges = [{ start, end, usedAt }, ...recentTimeRanges]
      .sort((a, b) => b.usedAt - a.usedAt)
      .slice(0, MAX_RECENT_TIME_RANGES);
  }

  localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(recentTimeRanges));
}
