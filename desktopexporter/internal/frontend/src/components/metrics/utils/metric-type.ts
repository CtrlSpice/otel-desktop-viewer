import type { MetricType } from '@/types/api-types'

const METRIC_TYPE_BADGE_BASE = 'badge badge-xs'
const METRIC_TYPE_BADGE_TONES = new Map<string, string>([
  ['Gauge', 'badge-info'],
  ['Sum', 'badge-secondary'],
  ['Histogram', 'badge-rose'],
  ['ExponentialHistogram', 'badge-warning'],
])

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
      return {
        label: 'Unspecified',
        title: 'Unspecified temporality',
        isUnspecified: true,
      }
    default:
      return { label: raw, title: raw, isUnspecified: false }
  }
}

const MONOTONIC_SYMBOL = { label: '↗', title: 'Monotonic' } as const

const UNSPECIFIED_TEMPORALITY_TITLE = 'aggregationTemporality = Unspecified'

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

  const labelParts = [metricTypeLabel(metricType)]
  if (temporality?.label) labelParts.push(temporality.label)
  if (isMonotonic === true) labelParts.push(MONOTONIC_SYMBOL.label)

  return {
    label: labelParts.join(' '),
    title: temporality?.isUnspecified
      ? `${titleParts.join(' · ')} · ${UNSPECIFIED_TEMPORALITY_TITLE}`
      : titleParts.join(' · '),
    className: temporality?.isUnspecified
      ? `${METRIC_TYPE_BADGE_BASE} badge-error`
      : `${METRIC_TYPE_BADGE_BASE} ${METRIC_TYPE_BADGE_TONES.get(metricType) ?? 'badge-neutral'}`,
  }
}
