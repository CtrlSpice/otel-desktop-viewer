// Heatmap colour ramp generator. Hand-tuned 8-stop sequential ramps, no
// interpolation. Three ramps keyed off `html[data-theme]`:
//   - rose-pine:      overlay (#26233a) through mauve into love (#eb6f92).
//   - rose-pine-moon: overlay (#393552) through mauve into love (#eb6f92).
//   - rose-pine-dawn: overlay (#f2e9e1) through rose mauve into amethyst (#907aa9).
// Visually monotonic steps so equal count bands read as equal visual
// steps -- the reason heatmaps work as a chart, not just decoration.

const MIN_STEPS = 1
// Cap matches the ramp length. Asking for more swatches would have to
// either repeat colours or interpolate; we explicitly chose 8 hand-tuned
// stops so we cap at that and let adaptiveStepCount round down.
const MAX_STEPS = 8

// 8-stop ramp for Rosé Pine OG (darkest). Overlay -> love.
const OG_RAMP: readonly string[] = [
  '#26233a',
  '#3d3460',
  '#5a3e72',
  '#7a4880',
  '#96528a',
  '#b45c90',
  '#d26592',
  '#eb6f92', // love (warm pink)
] as const

// 8-stop ramp for Rosé Pine Moon. Overlay -> love along
// a near-monochromatic violet-to-pink curve.
const MOON_RAMP: readonly string[] = [
  '#393552',
  '#4d3e63',
  '#654773',
  '#7f4f7f',
  '#9a5689',
  '#b55d8f',
  '#d16592',
  '#eb6f92', // love (warm pink)
] as const

// 8-stop ramp for Rosé Pine Dawn (light). Low counts near base surface;
// high counts deepen through dusty rose into amethyst.
const DAWN_RAMP: readonly string[] = [
  '#f2e9e1',
  '#ead7cf',
  '#e2c5c1',
  '#d9b3b6',
  '#cda2b0',
  '#be93ac',
  '#a985ab',
  '#907aa9', // amethyst
] as const

function rampForDataTheme(theme: string): readonly string[] {
  if (theme === 'rose-pine-dawn') return DAWN_RAMP
  if (theme === 'rose-pine') return OG_RAMP
  return MOON_RAMP
}

/**
 * Adaptive step count based on how many distinct count values appear in
 * the heatmap data. Clamped to [MIN_STEPS, MAX_STEPS]. The cap matches
 * the ramp length so getHeatmapSwatches never needs to fabricate
 * intermediate colours.
 */
export function adaptiveStepCount(distinctCounts: number): number {
  if (!Number.isFinite(distinctCounts) || distinctCounts < MIN_STEPS) {
    return MIN_STEPS
  }
  if (distinctCounts > MAX_STEPS) return MAX_STEPS
  return Math.floor(distinctCounts)
}

/**
 * Build a sequential colour ramp for a heatmap.
 *
 * - `steps`: number of discrete swatches; pair with `adaptiveStepCount`
 *   to match the data's distinctness. Must be in [MIN_STEPS, MAX_STEPS].
 *
 * Returns a `string[]` of length `steps`, suitable for layerchart's
 * `cRange={...}` and the legend strip below the heatmap.
 *
 * Sampling strategy: when fewer than MAX_STEPS swatches are needed we
 * keep the first and last anchors and pick intermediate stops by
 * even-spaced index lookup into the theme ramp. No interpolation --
 * every returned colour is a verbatim hex from the source ramp.
 *
 * @param dataTheme - `document.documentElement.getAttribute('data-theme')`
 *   (e.g. from themeSignal). Unknown / empty defaults to the moon ramp
 *   so SSR and pre-theme paint still look reasonable.
 */
export function getHeatmapSwatches(steps: number, dataTheme = ''): string[] {
  const ramp = rampForDataTheme(dataTheme)
  const safeSteps = Math.max(MIN_STEPS, Math.min(MAX_STEPS, Math.floor(steps)))
  if (safeSteps === ramp.length) return [...ramp]
  if (safeSteps === 1) return [ramp[ramp.length - 1]]
  const out: string[] = new Array(safeSteps)
  for (let i = 0; i < safeSteps; i++) {
    const srcIdx = Math.round((i * (ramp.length - 1)) / (safeSteps - 1))
    out[i] = ramp[srcIdx]
  }
  return out
}

/**
 * Compute legend bin edges for a quantize scale that maps [0, max] onto
 * `steps` colour swatches. Returns `steps + 1` numeric edges so callers
 * can format labels like "0 – 5 – 12 – 25" alongside the swatches.
 */
export function legendBinEdges(maxCount: number, steps: number): number[] {
  const safeSteps = Math.max(MIN_STEPS, Math.min(MAX_STEPS, Math.floor(steps)))
  const safeMax = Math.max(0, Number.isFinite(maxCount) ? maxCount : 0)
  const edges = new Array(safeSteps + 1)
  for (let i = 0; i <= safeSteps; i++) {
    edges[i] = (safeMax * i) / safeSteps
  }
  return edges
}
