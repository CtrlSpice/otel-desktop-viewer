import type { ChartPoint } from '@/types/metric-chart-types'

/**
 * Largest-Triangle-Three-Buckets downsampling (Steinarsson 2013).
 *
 * Thins a sorted-ascending array of chart points to at most `threshold`
 * entries while preserving visual shape — spikes survive because they
 * form the largest triangles.
 *
 * Every returned point is a real input sample (no interpolation).
 * If the input already fits within the budget, it's returned as-is
 * (no copy).
 */
export function downsampleLTTB(
  points: ChartPoint[],
  threshold: number,
): ChartPoint[] {
  const len = points.length
  if (threshold >= len || threshold < 3) return points

  const out: ChartPoint[] = new Array(threshold)

  // Always keep first and last.
  out[0] = points[0]
  out[threshold - 1] = points[len - 1]

  const bucketSize = (len - 2) / (threshold - 2)

  let prevSelectedIdx = 0

  for (let bucket = 0; bucket < threshold - 2; bucket++) {
    // Current bucket range (indices into `points`, skipping the
    // already-selected first element).
    const buckStart = Math.floor(bucket * bucketSize) + 1
    const buckEnd = Math.min(
      Math.floor((bucket + 1) * bucketSize) + 1,
      len - 1,
    )

    // Next bucket average (the "C" vertex of each candidate triangle).
    const nextBuckStart = buckEnd
    const nextBuckEnd = Math.min(
      Math.floor((bucket + 2) * bucketSize) + 1,
      len - 1,
    )
    let avgX = 0
    let avgY = 0
    const nextBuckLen = nextBuckEnd - nextBuckStart
    for (let j = nextBuckStart; j < nextBuckEnd; j++) {
      avgX += points[j].date.getTime()
      avgY += points[j].value
    }
    avgX /= nextBuckLen
    avgY /= nextBuckLen

    // "A" vertex: the previously selected point.
    const ax = points[prevSelectedIdx].date.getTime()
    const ay = points[prevSelectedIdx].value

    // Pick the point in the current bucket that maximises the
    // triangle area with A and avg(next bucket).
    let maxArea = -1
    let bestIdx = buckStart
    for (let j = buckStart; j < buckEnd; j++) {
      const area = Math.abs(
        (ax - avgX) * (points[j].value - ay) -
          (ax - points[j].date.getTime()) * (avgY - ay),
      )
      if (area > maxArea) {
        maxArea = area
        bestIdx = j
      }
    }

    out[bucket + 1] = points[bestIdx]
    prevSelectedIdx = bestIdx
  }

  return out
}
