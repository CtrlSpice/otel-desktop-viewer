import { SpanData } from "../types/api-types";
import { PreciseTimestamp } from "../types/precise-timestamp";

export function getTraceBounds(spans: SpanData[]): { startTime: PreciseTimestamp; endTime: PreciseTimestamp } {
  if (!spans.length) {
    return { startTime: new PreciseTimestamp(BigInt(0)), endTime: new PreciseTimestamp(BigInt(0)) };
  }

  let earliestStart = spans[0].startTime.nanoseconds;
  let latestEnd = spans[0].endTime.nanoseconds;

  spans.forEach((span) => {
    let spanStart = span.startTime.nanoseconds;
    if (spanStart < earliestStart) {
      earliestStart = spanStart;
    }

    let spanEnd = span.endTime.nanoseconds;
    if (spanEnd > latestEnd) {
      latestEnd = spanEnd;
    }
  });

  return { startTime: new PreciseTimestamp(earliestStart), endTime: new PreciseTimestamp(latestEnd) };
}

export function formatDuration(nanoseconds: bigint): string {
  if (nanoseconds >= BigInt(1_000_000_000)) {
    // Convert to seconds
    let seconds = Number(nanoseconds) / 1_000_000_000;
    return `${seconds.toFixed(3)} s`;
  } else if (nanoseconds >= BigInt(1_000_000)) {
    // Show milliseconds
    let ms = Number(nanoseconds) / 1_000_000;
    return `${ms.toFixed(3)} ms`;
  } else if (nanoseconds >= BigInt(1000)) {
    // Show microseconds
    let μs = Number(nanoseconds) / 1000;
    return `${μs.toFixed(3)} μs`;
  } else {
    // Show nanoseconds
    return `${Number(nanoseconds)} ns`;
  }
}

export function getOffset(startTime: PreciseTimestamp, endTime: PreciseTimestamp, point: PreciseTimestamp): number {
  let totalNs = endTime.nanoseconds - startTime.nanoseconds;
  let offsetNs = point.nanoseconds - startTime.nanoseconds;
  return Math.floor(Number((offsetNs * BigInt(100)) / totalNs));
}


