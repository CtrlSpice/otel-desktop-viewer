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

export function getOffset(
  startTime: bigint,
  endTime: bigint,
  point: bigint
): number {
  let totalNs = endTime - startTime;
  let offsetNs = point - startTime;
  return Math.floor(Number((offsetNs * BigInt(100)) / totalNs));
}
