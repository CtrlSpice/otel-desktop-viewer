import { heatmapSwatches } from '@/utils/chart-palette'

const MIN_STEPS = 1
const MAX_STEPS = 12

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

/** Match distinct non-zero count values to a ramp length in [1, 12]. */
export function adaptiveStepCount(distinctCounts: number): number {
  if (!Number.isFinite(distinctCounts) || distinctCounts < MIN_STEPS) {
    return MIN_STEPS
  }
  if (distinctCounts > MAX_STEPS) return MAX_STEPS
  return Math.floor(distinctCounts)
}

/** Equal-width integer bands below a separate maximum-count band. */
export function heatmapCountThresholds(
  maxCount: number,
  swatchSteps: number
): number[] {
  if (swatchSteps <= 0 || maxCount <= 0) return []
  if (!Number.isInteger(swatchSteps) || swatchSteps >= 2 ** 32) {
    throw new RangeError('swatchSteps must be a valid array length')
  }
  if (swatchSteps === 1) return [maxCount]
  const width = Math.ceil((maxCount - 1) / Math.min(swatchSteps, MAX_STEPS - 1))
  const out: number[] = []
  for (let lower = 1; lower < maxCount; lower += width) {
    out.push(lower)
  }
  out.push(maxCount)
  return out
}

function heatmapLegendLabel(
  bandIndex: number,
  swatchSteps: number,
  maxCount: number,
  thresholds: number[]
): string {
  const lo = Math.ceil(thresholds[bandIndex]!)
  const hi =
    bandIndex === swatchSteps - 1
      ? maxCount
      : Math.floor(thresholds[bandIndex + 1]! - 1e-9)
  return lo === hi ? String(lo) : `${lo}\u2009–\u2009${hi}`
}

/** Shared count ramp for heatmap cells and legend swatches. */
export function computeHeatmapColorScale(opts: {
  maxCount: number
  distinctNonZeroCount: number
  theme?: string
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

  const targetSteps = Math.min(
    adaptiveStepCount(opts.distinctNonZeroCount),
    Math.floor(opts.maxCount)
  )
  const thresholds = heatmapCountThresholds(opts.maxCount, targetSteps)
  const swatchSteps = thresholds.length
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
