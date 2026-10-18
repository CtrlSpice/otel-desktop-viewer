import type { ChartStickySize } from './chart-sticky-position'

type Rect = { left: number; top: number; width: number; height: number }

/** Keep the card near its bar, preferring clear space over covering counts. */
export function histogramCardPosition({
  width,
  height,
  cardWidth,
  cardHeight,
  selected,
  plot,
  bars,
}: ChartStickySize & {
  selected: Rect
  plot: Rect
  bars: readonly Rect[]
}) {
  const inset = 8
  const gap = 12
  const minLeft = Math.min(
    Math.max(inset, plot.left),
    Math.max(inset, width - cardWidth - inset)
  )
  const maxLeft = Math.max(
    minLeft,
    Math.min(width - cardWidth - inset, plot.left + plot.width - cardWidth)
  )
  const minTop = Math.min(
    Math.max(inset, plot.top),
    Math.max(inset, height - cardHeight - inset)
  )
  const maxTop = Math.max(
    minTop,
    Math.min(height - cardHeight - inset, plot.top + plot.height - cardHeight)
  )
  const clampLeft = (left: number) => Math.max(minLeft, Math.min(maxLeft, left))
  const clampTop = (top: number) => Math.max(minTop, Math.min(maxTop, top))
  const anchorX = selected.left + selected.width / 2
  const preferredTop = clampTop(selected.top - cardHeight / 2)
  const lefts = new Set([
    clampLeft(selected.left + selected.width + gap),
    clampLeft(selected.left - gap - cardWidth),
    clampLeft(anchorX - cardWidth / 2),
  ])

  let best = { left: minLeft, top: preferredTop }
  let bestOverlap = Infinity
  let bestDistance = Infinity
  for (const left of lefts) {
    // All bars rise from one baseline: above the tallest intersecting bar is
    // the nearest completely clear vertical position for this horizontal slot.
    const intersecting = bars.filter(
      bar =>
        bar.height > 0 &&
        bar.left < left + cardWidth + gap &&
        bar.left + bar.width > left - gap
    )
    const ceiling = intersecting.reduce(
      (top, bar) => Math.min(top, bar.top - gap - cardHeight),
      preferredTop
    )
    for (const top of new Set([preferredTop, clampTop(ceiling)])) {
      const overlap = intersecting.reduce(
        (area, bar) =>
          area +
          Math.max(
            0,
            Math.min(left + cardWidth, bar.left + bar.width + gap) -
              Math.max(left, bar.left - gap)
          ) *
            Math.max(
              0,
              Math.min(top + cardHeight, bar.top + bar.height + gap) -
                Math.max(top, bar.top - gap)
            ),
        0
      )
      const distance =
        (left + cardWidth / 2 - anchorX) ** 2 +
        (top + cardHeight / 2 - selected.top) ** 2
      if (
        overlap < bestOverlap ||
        (overlap === bestOverlap && distance < bestDistance)
      ) {
        best = { left, top }
        bestOverlap = overlap
        bestDistance = distance
      }
    }
  }
  return best
}
