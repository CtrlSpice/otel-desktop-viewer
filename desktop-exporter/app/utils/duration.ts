import Timestamp from "timestamp-nano";
import { SpanData } from "../types/api-types";

export function getSpanDuration(startTimestamp: string, endTimestamp: string) {
  try {
    let startTimeMs = Date.parse(startTimestamp.split(".")[0]);
    let endTimeMs = Date.parse(endTimestamp.split(".")[0]);

    let startTimeNs =
      startTimeMs * 1e6 + Timestamp.fromString(startTimestamp).getNano();
    let endTimeNs =
      endTimeMs * 1e6 + Timestamp.fromString(endTimestamp).getNano();

    return endTimeNs - startTimeNs;
  } catch (error) {
    return null;
  }
}

export function getSpanDurationString(
  startTimestamp: string,
  endTimestamp: string,
) {
  let durationNs = getSpanDuration(startTimestamp, endTimestamp);

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

export function getTraceDurationNs(spans: SpanData[]) {
  if (!spans.length) {
    return 0;
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

  return latestEndTime - earliestStartTime;
}

export function getNsFromString(timestampString: string) {
  let milliseconds = Date.parse(timestampString.split(".")[0]);
  let nanoseconds =
    milliseconds * 1e6 + Timestamp.fromString(timestampString).getNano();
  return nanoseconds;
}