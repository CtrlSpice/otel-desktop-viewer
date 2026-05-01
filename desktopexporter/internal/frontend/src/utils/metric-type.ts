import type { MetricType } from '@/types/api-types'

const METRIC_TYPE_BADGE_CLASS: Record<string, string> = {
  Gauge: 'badge badge-xs badge-soft badge-info',
  Sum: 'badge badge-xs badge-soft badge-primary',
  Histogram: 'badge badge-xs badge-soft badge-warning',
  ExponentialHistogram: 'badge badge-xs badge-soft badge-secondary',
  Empty: 'badge badge-xs badge-soft badge-neutral',
}

/** Stroke/fill color for charts — matches `METRIC_TYPE_BADGE_CLASS` semantics */
const METRIC_TYPE_SERIES_COLOR: Record<string, string> = {
  Gauge: 'var(--color-info)',
  Sum: 'var(--color-primary)',
  Histogram: 'var(--color-warning)',
  ExponentialHistogram: 'var(--color-secondary)',
  Empty: 'var(--color-neutral)',
}

export function metricTypeBadgeClass(
  metricType: MetricType | string
): string {
  return METRIC_TYPE_BADGE_CLASS[metricType] ?? METRIC_TYPE_BADGE_CLASS.Empty
}

export function metricTypeSeriesColor(metricType: MetricType | string): string {
  return METRIC_TYPE_SERIES_COLOR[metricType] ?? METRIC_TYPE_SERIES_COLOR.Empty
}

export function metricTypeLabel(metricType: string): string {
  if (metricType === 'ExponentialHistogram') return 'ExpHist'
  return metricType
}
