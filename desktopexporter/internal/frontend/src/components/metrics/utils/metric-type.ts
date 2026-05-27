import type { MetricType } from '@/types/api-types'
import {
  categoricalPalette,
  type CategoricalStem,
} from '@/utils/chart-palette'
import { themeSignal } from '@/state/theme.svelte'

// Single source of truth: metric type → categorical palette stem. Drives both
// the badge tone (via STEM_TO_BADGE) and the chart series colour (via
// categoricalPalette). Adding a new metric type? Add it here and everything
// downstream picks it up.
const METRIC_TYPE_STEM: Record<string, CategoricalStem> = {
  Gauge: 'foam',
  Sum: 'pine',
  Histogram: 'rose',
  ExponentialHistogram: 'gold',
}

const STEM_TO_BADGE: Record<CategoricalStem, string> = {
  pine: 'badge-secondary',
  foam: 'badge-info',
  gold: 'badge-warning',
  rose: 'badge-rose',
  iris: 'badge-primary',
}

const METRIC_TYPE_BADGE_BASE = 'badge badge-xs badge-soft'

/** Categorical palette stem for a metric type. Always returns a stem so
 *  callers building a palette array (legend + chart) get aligned indices
 *  even when the metric type is briefly `'Empty'` mid-load -- otherwise
 *  the chart and legend would have to guard `null` independently and
 *  risk picking different fallbacks. `'foam'` is an arbitrary cool-end
 *  pick for the unknown case; once the real metric type arrives the
 *  palette settles to that type's stem. Badge tone uses the raw lookup
 *  (without this fallback) so unknown types still read as neutral. */
export function metricTypeStem(
  metricType: MetricType | string
): CategoricalStem {
  return METRIC_TYPE_STEM[metricType] ?? 'foam'
}

export function metricTypeBadgeTone(metricType: MetricType | string): string {
  const stem = METRIC_TYPE_STEM[metricType]
  return stem ? STEM_TO_BADGE[stem] : 'badge-neutral'
}

export function metricTypeBadgeClass(
  metricType: MetricType | string
): string {
  return `${METRIC_TYPE_BADGE_BASE} ${metricTypeBadgeTone(metricType)}`
}

/** Single-colour chart fill for a metric type (e.g. histogram bars). Pulls
 *  slot 0 of the categorical palette starting at the metric type's stem,
 *  so it stays in lockstep with the series palette used by line charts.
 *  Unknown metric types get neutral (no palette fallback) -- a single fill
 *  shouldn't lie about which type is rendering. */
export function metricTypeSeriesColor(metricType: MetricType | string): string {
  const stem = METRIC_TYPE_STEM[metricType]
  if (!stem) return 'var(--color-neutral)'
  return categoricalPalette(1, stem, themeSignal.value)[0]
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
      return { label: 'Unspecified', title: 'Unspecified temporality', isUnspecified: true }
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
      : metricTypeBadgeClass(metricType),
  }
}
