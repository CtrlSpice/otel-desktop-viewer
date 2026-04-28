import type { MetricType } from '@/types/api-types'

const METRIC_TYPE_BADGE_CLASS: Record<string, string> = {
  Gauge: 'badge badge-xs badge-soft badge-info',
  Sum: 'badge badge-xs badge-soft badge-success',
  Histogram: 'badge badge-xs badge-soft badge-warning',
  ExponentialHistogram: 'badge badge-xs badge-soft badge-secondary',
  Empty: 'badge badge-xs badge-soft badge-neutral',
}

export function metricTypeBadgeClass(
  metricType: MetricType | string
): string {
  return METRIC_TYPE_BADGE_CLASS[metricType] ?? METRIC_TYPE_BADGE_CLASS.Empty
}

export function metricTypeLabel(metricType: string): string {
  if (metricType === 'ExponentialHistogram') return 'ExpHist'
  return metricType
}
