import { SpanData } from "../types/api-types";
import { PreciseTimestamp } from "../types/precise-timestamp";

export type Duration = {
  startTime: PreciseTimestamp
  endTime: PreciseTimestamp
  milliseconds: number
  nanoseconds: number
  label: string
};

export function getTraceDuration(spans: SpanData[]): Duration {
  if (!spans.length) {
    return {
      startTime: new PreciseTimestamp(0, 0),
      endTime: new PreciseTimestamp(0, 0),
      milliseconds: 0,
      nanoseconds: 0,
      label: ""
    };
  }

  let earliestStartTime = spans[0].startTime;
  let latestEndTime = spans[0].endTime;

  spans.forEach((span) => {
    let spanStart = span.startTime;
    if (spanStart.isBefore(earliestStartTime)) {
      earliestStartTime = spanStart;
    }

    let spanEnd = span.endTime;
    if (spanEnd.isAfter(latestEndTime)) {
      latestEndTime = spanEnd;
    }
  });

  return getDuration(earliestStartTime, latestEndTime);
}

export function getDuration(startTime: PreciseTimestamp, endTime: PreciseTimestamp): Duration {
  if (startTime === null || endTime === null) {
    return {
      startTime: startTime,
      endTime: endTime,
      milliseconds: 0,
      nanoseconds: 0,
      label: ""
    };
  }

  // Calculate duration in milliseconds and nanoseconds separately
  let msDiff = endTime.milliseconds - startTime.milliseconds;
  let nsDiff = endTime.nanoseconds - startTime.nanoseconds;

  // If nanoseconds went negative, borrow from milliseconds
  let finalMs = msDiff;
  let finalNs = nsDiff;
  if (nsDiff < 0) {
    finalMs -= 1;
    finalNs += 1e6; // Add 1ms worth of nanoseconds
  }

  // Format label based on total duration
  let label = "";
  if (finalMs >= 1000) {
    // Convert to seconds
    let seconds = finalMs / 1000;
    label = `${seconds.toFixed(3)} s`
  } else if (finalMs >= 1) {
    // Show milliseconds with nanosecond precision
    label = `${finalMs}.${finalNs.toString().padStart(6, '0')} ms`
  } else if (finalNs >= 1000) {
    // Show microseconds
    label = `${(finalNs / 1000).toFixed(3)} μs`
  } else {
    // Show nanoseconds
    label = `${finalNs} ns`
  }

  return {
    startTime: startTime,
    endTime: endTime,
    milliseconds: finalMs,
    nanoseconds: finalNs,
    label: label
  };
}

export function getOffset(duration: Duration, point: PreciseTimestamp): number {
  // Try nanosecond precision first
  let totalNs = duration.milliseconds * 1e6 + duration.nanoseconds;
  let offsetNs = (point.milliseconds - duration.startTime.milliseconds) * 1e6 + 
                 (point.nanoseconds - duration.startTime.nanoseconds);

  if (totalNs <= Number.MAX_SAFE_INTEGER && offsetNs <= Number.MAX_SAFE_INTEGER) {
    // Use nanosecond precision
    return Math.floor((offsetNs / totalNs) * 100);
  } else {
    // Fall back to millisecond precision
    return Math.floor(((point.milliseconds - duration.startTime.milliseconds) / 
                      (duration.endTime.milliseconds - duration.startTime.milliseconds)) * 100);
  }
}


