export type Timezone = 'local' | 'UTC';

type DateTimeResolution = 'minutes' | 'seconds' | 'milliseconds';
type TimestampResolution = DateTimeResolution | 'microseconds' | 'nanoseconds';

// UI time: number = Unix ms (from Date.now(), time pickers, etc.)
export function formatDateTime(
  ms: number,
  timezone: Timezone,
  resolution: DateTimeResolution = 'minutes'
): string {
  return formatWithDate(new Date(ms), timezone, resolution);
}

// Telemetry time: bigint = Unix nanoseconds (from backend OTLP data)
export function formatTimestamp(
  ns: bigint,
  timezone: Timezone,
  resolution: TimestampResolution = 'nanoseconds'
): string {
  let epochMs = Number(ns / 1_000_000n);
  let subMs = ns % 1_000_000n;
  let date = new Date(epochMs);
  let formatted = formatWithDate(date, timezone, resolution);

  if (resolution === 'microseconds') {
    let micros = Number(subMs).toString().padStart(6, '0');
    return formatted.replace(/\.\d{3}(\s)/, `.${micros}$1`);
  }
  if (resolution === 'nanoseconds') {
    let nanos = Number(subMs).toString().padStart(6, '0');
    let extraNanos = Number(ns % 1000n).toString().padStart(3, '0');
    return formatted.replace(/\.\d{3}(\s)/, `.${nanos}${extraNanos}$1`);
  }
  return formatted;
}

function formatWithDate(
  date: Date,
  timezone: Timezone,
  resolution: TimestampResolution
): string {
  let options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  };

  switch (resolution) {
    case 'seconds':
      options.second = '2-digit';
      break;
    case 'milliseconds':
    case 'microseconds':
    case 'nanoseconds':
      options.second = '2-digit';
      options.fractionalSecondDigits = 3;
      break;
  }

  let formattedDate: string;
  if (timezone === 'UTC') {
    formattedDate = date.toLocaleString('en-CA', { ...options, timeZone: 'UTC' });
  } else {
    formattedDate = date.toLocaleString('en-CA', options);
  }

  if (timezone === 'UTC') {
    return `${formattedDate} UTC`;
  }

  let tzAbbr =
    new Intl.DateTimeFormat('en', { timeZoneName: 'short' })
      .formatToParts(date)
      .find(part => part.type === 'timeZoneName')?.value || '';
  return `${formattedDate} ${tzAbbr}`;
}

export function formatDateTimeRange(
  start: number,
  end: number,
  timezone: Timezone
): string {
  // Handle "Show all" case where start is 0 (beginning of time)
  if (start === 0) {
    return `Before ${formatDateTime(end, timezone, 'seconds')}`;
  }

  let startStr = formatDateTime(start, timezone, 'seconds');
  let endStr = formatDateTime(end, timezone, 'seconds');

  // Extract date and time parts for reuse
  let startParts = startStr.split(' ');
  let endParts = endStr.split(' ');
  let timezoneSuffix = startParts[2] ?? '';
  let isSameDay = startParts[0] === endParts[0];

  startStr = startStr.replace(timezoneSuffix, '');
  endStr = endStr.replace(timezoneSuffix, '');

  if (isSameDay) {
    // Same day: "2024-01-15 14:30:45 - 15:45:30 UTC"
    return `${startStr} - ${endParts[1]} ${timezoneSuffix}`;
  } else {
    // Different days: "2024-01-15 14:30:45 - 2024-01-16 09:15:30 UTC"
    return `${startStr} - ${endStr} ${timezoneSuffix}`;
  }
}

export function getLocalTimezoneName(): string {
  try {
    let timeZoneName = new Intl.DateTimeFormat('en', {
      timeZoneName: 'long',
    })
      .formatToParts(new Date())
      .find(part => part.type === 'timeZoneName')?.value;

    return timeZoneName || 'Local Time';
  } catch (error) {
    return 'Local Time';
  }
}

// --- Duration formatting & parsing ---

import type { TraceSummary } from '@/types/api-types'

/**
 * Nanoseconds of span coverage for trace list display/sort.
 * Uses the root span when present; when the API starts filling summary from another span
 * (e.g. earliest) if root is missing, centralize that choice here.
 */
export function traceSummaryDurationNs(
  summary: TraceSummary
): bigint | undefined {
  const span = summary.rootSpan
  if (!span) return undefined
  const ns = span.endTime - span.startTime
  return ns >= 0n ? ns : undefined
}

const DURATION_UNITS: Record<string, bigint> = {
  ns: 1n,
  us: 1_000n,
  '\u00b5s': 1_000n, // µs
  ms: 1_000_000n,
  s: 1_000_000_000n,
  m: 60_000_000_000n,
  min: 60_000_000_000n,
  h: 3_600_000_000_000n,
}

const DURATION_RE = /^(\d+(?:\.\d+)?)\s*(ns|us|µs|ms|s|min|m|h)$/i

/**
 * Parse a human-readable duration string into nanoseconds.
 * Accepts formats like "1s", "500ms", "2m", "1.5h", "100ns".
 * Plain numeric strings are treated as raw nanoseconds.
 * Returns null if the string cannot be parsed.
 */
export function parseDuration(input: string): bigint | null {
  const trimmed = input.trim()
  if (!trimmed) return null

  if (/^\d+$/.test(trimmed)) return BigInt(trimmed)

  const match = trimmed.match(DURATION_RE)
  if (!match) return null

  const [, numStr, unit] = match
  const multiplier = DURATION_UNITS[unit.toLowerCase()]
  if (multiplier === undefined) return null

  const num = parseFloat(numStr)
  if (!isFinite(num) || num < 0) return null

  if (Number.isInteger(num)) {
    return BigInt(num) * multiplier
  }
  return BigInt(Math.round(num * Number(multiplier)))
}

export function formatDuration(nanoseconds: bigint): string {
  if (nanoseconds >= 1_000_000_000n) {
    let seconds = Number(nanoseconds) / 1_000_000_000;
    return `${seconds.toFixed(3)} s`;
  } else if (nanoseconds >= 1_000_000n) {
    let ms = Number(nanoseconds) / 1_000_000;
    return `${ms.toFixed(3)} ms`;
  } else if (nanoseconds >= 1000n) {
    let μs = Number(nanoseconds) / 1000;
    return `${μs.toFixed(3)} μs`;
  } else {
    return `${Number(nanoseconds)} ns`;
  }
}

export function getOffset(
  startTime: bigint,
  endTime: bigint,
  point: bigint
): number {
  let totalNs = endTime - startTime;
  if (totalNs <= 0n) return 0;
  let offsetNs = point - startTime;
  return Math.floor(Number((offsetNs * 100n) / totalNs));
}

// --- Recent time ranges (localStorage persistence) ---

const RECENT_STORAGE_KEY = 'datetime-filter-recent';

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
