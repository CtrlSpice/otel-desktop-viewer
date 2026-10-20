import {
  autoTickVals,
  filterTicksByFormat,
  isScaleBand,
  isScaleUtc,
  type AnyScale,
  type AxisProps,
} from 'layerchart'

/** Match LayerChart's Cartesian axis tick selection and band-centre positions. */
export function metricAxisTickPositions(
  scale: AnyScale,
  size: number,
  axis: Omit<AxisProps, 'placement'>,
  defaultSpacing: number
): number[] {
  const spacing = axis.tickSpacing ?? (isScaleBand(scale) ? null : defaultSpacing)
  const count = typeof axis.ticks === 'number'
    ? axis.ticks
    : spacing ? Math.round(size / spacing) : undefined
  const ticks = filterTicksByFormat(autoTickVals(scale, axis.ticks, count), axis.format, {
    utc: isScaleUtc(scale),
  })
  const offset = isScaleBand(scale) ? scale.bandwidth() / 2 : 0
  return [...new Set(ticks.map(tick => Number(scale(tick)) + offset))]
}

/** Two equal divisions per labelled interval, continued to the plot edges. */
export function metricDotPositions(ticks: readonly number[], size: number): number[] {
  const anchors = [...new Set(ticks)].sort((a, b) => a - b)
  if (anchors.length < 2) return anchors.filter(value => value >= 0 && value <= size)

  const positions: number[] = []
  const first = anchors[0]!
  const firstStep = (anchors[1]! - first) / 2
  for (let index = Math.floor(first / firstStep); index > 0; index--) {
    positions.push(first - index * firstStep)
  }
  for (let index = 0; index < anchors.length - 1; index++) {
    const start = anchors[index]!
    const step = (anchors[index + 1]! - start) / 2
    for (let half = 0; half < 2; half++) {
      positions.push(start + half * step)
    }
  }
  const last = anchors.at(-1)!
  const lastStep = (last - anchors.at(-2)!) / 2
  for (let index = 0; index <= Math.floor((size - last) / lastStep); index++) {
    positions.push(last + index * lastStep)
  }
  return positions.filter(value => value >= 0 && value <= size)
}

/** Continue one step past the chosen edge, fading its three outer positions. */
export function metricDotGuides(
  ticks: readonly number[], size: number, edge: 'start' | 'end'
) {
  const positions = metricDotPositions(ticks, size)
  if (positions.length < 2) return positions.map(position => ({ position, opacity: 1 }))

  if (edge === 'start') {
    const first = positions[0]!
    positions.unshift(first - (positions[1]! - first))
  } else {
    const last = positions.at(-1)!
    positions.push(last + (last - positions.at(-2)!))
  }
  return positions.map((position, index) => ({
    position,
    opacity: Math.min(1, (edge === 'start' ? index : positions.length - index - 1) / 3),
  }))
}
