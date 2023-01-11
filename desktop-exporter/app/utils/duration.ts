import Timestamp from "timestamp-nano";
import { SpanData } from "../types/api-types";

export type TraceTimeAttributes = {
  traceStartTimeNS: number;
  traceDurationNS: number;
};

export function getTraceTimeAttributes(spans: SpanData[]): TraceTimeAttributes {
  if (!spans.length) {
    return {
      traceStartTimeNS: 0,
      traceDurationNS: 0,
    };
  }

  let earliestStartTime = NsFromString(spans[0].startTime);
  let latestEndTime = NsFromString(spans[0].endTime);

  spans.forEach((span) => {
    let spanStart = NsFromString(span.startTime);
    if (spanStart < earliestStartTime) {
      earliestStartTime = spanStart;
    }

    let spanEnd = NsFromString(span.endTime);
    if (spanEnd > latestEndTime) {
      latestEndTime = spanEnd;
    }
  });

  return {
    traceStartTimeNS: earliestStartTime,
    traceDurationNS: latestEndTime - earliestStartTime,
  };
}

export function NsFromString(timestampString: string) {
  let milliseconds = Date.parse(timestampString.split(".")[0]);
  let nanoseconds =
    milliseconds * 1e6 + Timestamp.fromString(timestampString).getNano();
  return nanoseconds;
}

export function DurationNs(startTimestamp: string, endTimestamp: string) {
  let startTimeNs = NsFromString(startTimestamp);
  let endTimeNs = NsFromString(endTimestamp);

  return endTimeNs - startTimeNs;
}

export function DurationString(durationNs: number) {
  if (durationNs === null || durationNs < 0) {
    return null;
  }

  // Label in seconds
  if (durationNs >= 1e9) {
    return `${durationNs / 1e9} s`;
  }

  // Label in milliseconds
  if (durationNs >= 1e6) {
    return `${durationNs / 1e6} ms`;
  }

  // Label in microseconds
  if (durationNs >= 1e3) {
    return `${durationNs / 1e3} μs`;
  }

  // Label in nanoseconds
  return `${durationNs} ns`;
}


