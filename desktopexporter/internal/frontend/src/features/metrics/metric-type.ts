import type { MetricType } from '@/types/api-types'

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

export type MetricTypeSummary = {
  label: string
  title: string
}

/** Summary type label with Δ / Σ / ↗ suffixes and the full meaning in its title. */
export function metricTypeSummary(
  metricType: MetricType | string,
  aggregationTemporality: string | null | undefined,
  isMonotonic: boolean | null
): MetricTypeSummary {
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
  }
}
