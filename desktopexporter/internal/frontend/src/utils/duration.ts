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
