import type { MetricSummary } from '@/types/api-types'

type MetricTypeAxes = Pick<
  MetricSummary,
  'metricType' | 'aggregationTemporalityCode' | 'isMonotonic'
>

function optionalNumber(
  a: number | null | undefined,
  b: number | null | undefined
): number {
  if (a == null) return b == null ? 0 : 1
  if (b == null) return -1
  return a - b
}

// Ascending: type name; Delta, Cumulative, Unspecified, unknown codes, missing;
// Sum only: non-monotonic, monotonic, missing. Inapplicable axes are ignored.
export function compareMetricType(
  a: MetricTypeAxes,
  b: MetricTypeAxes
): number {
  const type = a.metricType.localeCompare(b.metricType)
  if (type) return type
  if (!['Sum', 'Histogram', 'ExponentialHistogram'].includes(a.metricType))
    return 0
  const rank = (code: number | null | undefined) =>
    code == null ? null : code === 1 ? 1 : code === 2 ? 2 : code === 0 ? 3 : 4
  const temporality =
    optionalNumber(
      rank(a.aggregationTemporalityCode),
      rank(b.aggregationTemporalityCode)
    ) ||
    optionalNumber(a.aggregationTemporalityCode, b.aggregationTemporalityCode)
  if (temporality) return temporality
  return a.metricType === 'Sum'
    ? optionalNumber(
        a.isMonotonic == null ? null : Number(a.isMonotonic),
        b.isMonotonic == null ? null : Number(b.isMonotonic)
      )
    : 0
}
