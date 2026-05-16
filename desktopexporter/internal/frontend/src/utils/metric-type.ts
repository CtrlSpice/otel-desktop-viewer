import type { MetricType } from '@/types/api-types'

// TODO: unify metric-type badge colours — currently info and accent
// resolve to the same hex in both Rosé Pine themes, so Gauge and Sum
// are visually indistinguishable. Pick a dedicated palette that gives
// each metric type a unique colour in both moon and dawn.
const METRIC_TYPE_BADGE_CLASS: Record<string, string> = {
  Gauge: 'badge badge-xs badge-soft badge-info',
  Sum: 'badge badge-xs badge-soft badge-info',
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
