import Timestamp from "timestamp-nano";

export function getDuration(startTimestamp: string, endTimestamp: string) {
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

export function getDurationString(
  startTimestamp: string,
  endTimestamp: string,
) {
  let durationNs = getDuration(startTimestamp, endTimestamp);

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
