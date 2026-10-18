export type ChartAnchor = { x: number; y: number }

/** Keep a pinned card near its mark, flipping sides before clamping to the plot. */
export function chartStickyPosition(
  anchor: ChartAnchor | null,
  width: number,
  height: number,
  cardWidth: number,
  cardHeight: number
) {
  const inset = 8
  const gap = 12
  const maxLeft = Math.max(inset, width - cardWidth - inset)
  const maxTop = Math.max(inset, height - cardHeight - inset)
  const clamp = (value: number, max: number) =>
    Math.min(max, Math.max(inset, value))
  if (!anchor || !Number.isFinite(anchor.x) || !Number.isFinite(anchor.y)) {
    return { left: maxLeft, top: inset }
  }
  let left = anchor.x + gap
  let top = anchor.y - cardHeight / 2
  if (left > maxLeft) {
    if (anchor.x - gap - cardWidth >= inset) {
      left = anchor.x - gap - cardWidth
    } else {
      left = anchor.x - cardWidth / 2
      top = anchor.y + gap
      if (top > maxTop) top = anchor.y - gap - cardHeight
    }
  }
  return { left: clamp(left, maxLeft), top: clamp(top, maxTop) }
}
