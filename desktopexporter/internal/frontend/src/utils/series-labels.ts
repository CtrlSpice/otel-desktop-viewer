import type { Attribute } from '@/types/api-types'
import type { MetricTimeseries } from '@/types/api-types'

/**
 * Works out which resource attributes distinguish the series of one metric.
 *
 * @remarks
 * Series split by the resource that emitted them, which is what stops two
 * replicas of a service interleaving into a single line. But replicas emit
 * *identical* labels — that is what made them collide in the first place — so
 * the legend has to show something from the resource or it renders two rows a
 * user cannot tell apart. That would be worse than the merged line splitting
 * replaced.
 *
 * Showing the whole resource is not the answer either: it is typically ~15
 * attributes, nearly all identical across the replicas, and the one that
 * matters (`host.name`, `k8s.pod.name`) is buried. So this returns only the
 * attributes whose values actually vary between the series.
 *
 * The common case is a metric whose series all share one resource, and there
 * the result is empty — no noise added to a legend that never needed it.
 *
 * @param timeseries - all series of one metric
 * @returns per-series distinguishing attributes, keyed by series id
 *
 * @example
 * Two pods of `checkout`, same labels, differing only in host.name:
 * `Map { seriesA => [host.name=pod-a], seriesB => [host.name=pod-b] }`
 */
export function distinguishingResourceAttributes(
  timeseries: MetricTimeseries[]
): Map<string, Attribute[]> {
  const result = new Map<string, Attribute[]>()
  if (timeseries.length < 2) {
    // A single series has nothing to be distinguished from.
    for (const ts of timeseries) result.set(ts.attributesKey, [])
    return result
  }

  // Collect every value each resource key takes across the series.
  const valuesByKey = new Map<string, Set<string>>()
  for (const ts of timeseries) {
    for (const attr of ts.resource?.attributes ?? []) {
      let seen = valuesByKey.get(attr.key)
      if (!seen) {
        seen = new Set()
        valuesByKey.set(attr.key, seen)
      }
      seen.add(attr.value)
    }
  }

  // A key varies if it has more than one value, or if it is absent from some
  // series -- absence is itself distinguishing.
  const varying = new Set<string>()
  for (const [key, values] of valuesByKey) {
    const presentEverywhere = timeseries.every(ts =>
      (ts.resource?.attributes ?? []).some(a => a.key === key)
    )
    if (values.size > 1 || !presentEverywhere) varying.add(key)
  }

  for (const ts of timeseries) {
    result.set(
      ts.attributesKey,
      (ts.resource?.attributes ?? []).filter(a => varying.has(a.key))
    )
  }
  return result
}
