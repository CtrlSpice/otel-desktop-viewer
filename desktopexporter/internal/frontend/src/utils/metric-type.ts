import type { MetricType } from '@/types/api-types'

// Metric-type badge tones: Gauge uses custom badge-rose (app.css).
const METRIC_TYPE_BADGE_BASE = 'badge badge-xs badge-soft'

const METRIC_TYPE_BADGE_TONE: Record<string, string> = {
  Gauge: 'badge-rose',
  Sum: 'badge-info',
  Histogram: 'badge-warning',
  ExponentialHistogram: 'badge-secondary',
  Empty: 'badge-neutral',
}

/** Stroke/fill color for charts — matches `METRIC_TYPE_BADGE_CLASS` semantics */
const METRIC_TYPE_SERIES_COLOR: Record<string, string> = {
  Gauge: 'var(--color-rose)',
  Sum: 'var(--color-primary)',
  Histogram: 'var(--color-warning)',
  ExponentialHistogram: 'var(--color-secondary)',
  Empty: 'var(--color-neutral)',
}

export function metricTypeBadgeTone(metricType: MetricType | string): string {
  return METRIC_TYPE_BADGE_TONE[metricType] ?? METRIC_TYPE_BADGE_TONE.Empty
}

export function metricTypeBadgeClass(
  metricType: MetricType | string
): string {
  return `${METRIC_TYPE_BADGE_BASE} ${metricTypeBadgeTone(metricType)}`
}

export function metricTypeSeriesColor(metricType: MetricType | string): string {
  return METRIC_TYPE_SERIES_COLOR[metricType] ?? METRIC_TYPE_SERIES_COLOR.Empty
}

export function metricTypeLabel(metricType: string): string {
  if (metricType === 'ExponentialHistogram') return 'ExpHist'
  return metricType
}

type TemporalityBadge = {
  label: string
  title: string
  isUnspecified: boolean
}

function temporalityBadge(
  aggregationTemporality: string | null | undefined
): TemporalityBadge | null {
  const raw = aggregationTemporality?.trim()
  if (!raw) return null

  switch (raw.toLowerCase()) {
    case 'delta':
      return { label: 'Δ', title: 'Delta', isUnspecified: false }
    case 'cumulative':
      return { label: 'Σ', title: 'Cumulative', isUnspecified: false }
    case 'unspecified':
      return { label: '', title: 'Unspecified temporality', isUnspecified: true }
    default:
      return { label: raw, title: raw, isUnspecified: false }
  }
}

const MONOTONIC_SYMBOL = { label: '↗', title: 'Monotonic' } as const

const UNSPECIFIED_TEMPORALITY_LABEL = 'aggregationTemporality = Unspecified'

export type MetricTypeCardBadge = {
  label: string
  title: string
  className: string
}

/** Drawer card: one type badge with Δ / Σ / ↗ suffixes; error tone when temporality is Unspecified. */
export function metricTypeCardBadge(
  metricType: MetricType | string,
  aggregationTemporality: string | null | undefined,
  isMonotonic: boolean | null
): MetricTypeCardBadge {
  const temporality = temporalityBadge(aggregationTemporality)

  const titleParts = [metricType]
  if (temporality) titleParts.push(temporality.title)
  if (isMonotonic === true) titleParts.push(MONOTONIC_SYMBOL.title)

  if (temporality?.isUnspecified) {
    return {
      label: UNSPECIFIED_TEMPORALITY_LABEL,
      title: titleParts.join(' · '),
      className: `${METRIC_TYPE_BADGE_BASE} badge-error`,
    }
  }

  const labelParts = [metricTypeLabel(metricType)]
  if (temporality?.label) labelParts.push(temporality.label)
  if (isMonotonic === true) labelParts.push(MONOTONIC_SYMBOL.label)

  return {
    label: labelParts.join(' '),
    title: titleParts.join(' · '),
    className: metricTypeBadgeClass(metricType),
  }
}
