import Timestamp from "timestamp-nano";
import { SpanData } from "../types/api-types";

export type TraceTiming = {
  traceStartTimeNS: number;
  traceDurationNS: number;
};

export function calculateTraceTiming(spans: SpanData[]): TraceTiming {
  if (!spans.length) {
    return {
      traceStartTimeNS: 0,
      traceDurationNS: 0,
    };
  }

  let earliestStartTime = getNsFromString(spans[0].startTime);
  let latestEndTime = getNsFromString(spans[0].endTime);

  spans.forEach((span) => {
    let spanStart = getNsFromString(span.startTime);
    if (spanStart < earliestStartTime) {
      earliestStartTime = spanStart;
    }

    let spanEnd = getNsFromString(span.endTime);
    if (spanEnd > latestEndTime) {
      latestEndTime = spanEnd;
    }
  });

  return {
    traceStartTimeNS: earliestStartTime,
    traceDurationNS: latestEndTime - earliestStartTime,
  };
}

export function getNsFromString(timestampString: string) {
  let milliseconds = Date.parse(timestampString.split(".")[0]);
  let nanoseconds =
    milliseconds * 1e6 + Timestamp.fromString(timestampString).getNano();
  return nanoseconds;
}

export function getDurationNs(startTimestamp: string, endTimestamp: string) {
  let startTimeNs = getNsFromString(startTimestamp);
  let endTimeNs = getNsFromString(endTimestamp);

  return endTimeNs - startTimeNs;
}

export function getDurationString(durationNs: number) {
  if (durationNs === null || durationNs < 0) {
    return "";
  }

  // Label in seconds
  if (durationNs >= 1e9) {
    return `${(durationNs / 1e9).toFixed(3)} s`;
  }

  // Label in milliseconds
  if (durationNs >= 1e6) {
    return `${(durationNs / 1e6).toFixed(3)} ms`;
  }

  // Label in microseconds
  if (durationNs >= 1e3) {
    return `${(durationNs / 1e3).toFixed(3)} μs`;
  }

  // Label in nanoseconds
  return `${durationNs} ns`;
}


