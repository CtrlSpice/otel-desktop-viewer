import { heatmapSwatches } from '@/utils/chart-palette'

const MIN_STEPS = 1
const MAX_STEPS = 8

export type HeatmapLegendEntry = {
  key: string
  label: string
  color: string
}

export type HeatmapColorScale = {
  swatchSteps: number
  thresholds: number[]
  swatches: string[]
  range: string[]
  legendEntries: HeatmapLegendEntry[]
}

/** Match distinct non-zero count values to a ramp length in [1, 8]. */
export function adaptiveStepCount(distinctCounts: number): number {
  if (!Number.isFinite(distinctCounts) || distinctCounts < MIN_STEPS) {
    return MIN_STEPS
  }
  if (distinctCounts > MAX_STEPS) return MAX_STEPS
  return Math.floor(distinctCounts)
}

/** scaleThreshold domain for positive heatmap counts (first edge is always 1). */
export function heatmapCountThresholds(
  maxCount: number,
  swatchSteps: number
): number[] {
  if (swatchSteps <= 0 || maxCount <= 0) return []
  const out: number[] = new Array(swatchSteps)
  out[0] = 1
  for (let i = 1; i < swatchSteps; i++) {
    const next = Math.ceil((i * maxCount) / swatchSteps)
    out[i] = next <= out[i - 1]! ? out[i - 1]! + 1 : next
  }
  return out
}

function heatmapLegendLabel(
  bandIndex: number,
  swatchSteps: number,
  maxCount: number,
  thresholds: number[]
): string {
  const lo = bandIndex === 0 ? 1 : Math.ceil(thresholds[bandIndex]!)
  const hi =
    bandIndex === swatchSteps - 1
      ? maxCount
      : Math.floor(thresholds[bandIndex + 1]! - 1e-9)
  const close = bandIndex === swatchSteps - 1 ? ']' : ')'
  return `(${lo}\u2009–\u2009${hi}${close}`
}

/** Shared count ramp for heatmap cells and footer legend swatches. */
export function computeHeatmapColorScale(opts: {
  maxCount: number
  distinctNonZeroCount: number
  theme: string
  emptySwatch?: string
}): HeatmapColorScale {
  const emptySwatch = opts.emptySwatch ?? 'var(--color-base-200)'
  if (opts.maxCount <= 0) {
    return {
      swatchSteps: 0,
      thresholds: [],
      swatches: [],
      range: [emptySwatch],
      legendEntries: [],
    }
  }

  const swatchSteps = Math.min(
    adaptiveStepCount(opts.distinctNonZeroCount),
    Math.floor(opts.maxCount)
  )
  const thresholds = heatmapCountThresholds(opts.maxCount, swatchSteps)
  const swatches = heatmapSwatches(swatchSteps, opts.theme)
  const range = [emptySwatch, ...swatches]

  const legendEntries: HeatmapLegendEntry[] = []
  for (let i = 0; i < swatches.length; i++) {
    const label = heatmapLegendLabel(i, swatchSteps, opts.maxCount, thresholds)
    legendEntries.push({
      key: label,
      label,
      color: swatches[i]!,
    })
  }

  return { swatchSteps, thresholds, swatches, range, legendEntries }
}
