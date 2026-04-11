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
    // Convert to seconds
    let seconds = Number(nanoseconds) / 1_000_000_000;
    return `${seconds.toFixed(3)} s`;
  } else if (nanoseconds >= 1_000_000n) {
    // Show milliseconds
    let ms = Number(nanoseconds) / 1_000_000;
    return `${ms.toFixed(3)} ms`;
  } else if (nanoseconds >= 1000n) {
    // Show microseconds
    let μs = Number(nanoseconds) / 1000;
    return `${μs.toFixed(3)} μs`;
  } else {
    // Show nanoseconds
    return `${Number(nanoseconds)} ns`;
  }
}

export function getOffset(
  startTime: bigint,
  endTime: bigint,
  point: bigint
): number {
  let totalNs = endTime - startTime;
  let offsetNs = point - startTime;
  return Math.floor(Number((offsetNs * 100n) / totalNs));
}
