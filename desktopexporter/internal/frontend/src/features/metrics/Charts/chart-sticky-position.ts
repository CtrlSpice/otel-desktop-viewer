export type ChartAnchor = { x: number; y: number }
export type ChartObstacle = {
  left: number
  top: number
  width: number
  height: number
}
export type ChartStickySize = {
  width: number
  height: number
  cardWidth: number
  cardHeight: number
}

/** Keep a pinned card near its mark, avoiding supplied label/marker rectangles. */
export function chartStickyPosition(
  anchor: ChartAnchor | null,
  width: number,
  height: number,
  cardWidth: number,
  cardHeight: number,
  obstacles: readonly ChartObstacle[] = []
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
  const preferred = { left: clamp(left, maxLeft), top: clamp(top, maxTop) }
  if (obstacles.length === 0) return preferred

  // Search obstacle edges as well as both sides of the anchor. The small
  // clearance keeps labels visually separate even when their boxes just fit.
  const clearance = 4
  const lefts = new Set([
    preferred.left,
    inset,
    maxLeft,
    clamp(anchor.x - gap - cardWidth, maxLeft),
  ])
  const tops = new Set([
    preferred.top,
    inset,
    maxTop,
    clamp(anchor.y + gap, maxTop),
    clamp(anchor.y - gap - cardHeight, maxTop),
  ])
  for (const obstacle of obstacles) {
    lefts.add(clamp(obstacle.left - clearance - cardWidth, maxLeft))
    lefts.add(clamp(obstacle.left + obstacle.width + clearance, maxLeft))
    tops.add(clamp(obstacle.top - clearance - cardHeight, maxTop))
    tops.add(clamp(obstacle.top + obstacle.height + clearance, maxTop))
  }
  let best = preferred
  let bestOverlap = Infinity
  let bestDistance = Infinity
  for (const candidateLeft of lefts) {
    for (const candidateTop of tops) {
      // Keep the card visibly attached to its point. A crowded neighbourhood
      // may retain overlap; distant whitespace is not a useful escape hatch.
      const anchorDistance = Math.hypot(
        Math.max(
          candidateLeft - anchor.x,
          anchor.x - candidateLeft - cardWidth,
          0
        ),
        Math.max(
          candidateTop - anchor.y,
          anchor.y - candidateTop - cardHeight,
          0
        )
      )
      if (anchorDistance > 48) continue
      const overlap = obstacles.reduce(
        (area, obstacle) =>
          area +
          Math.max(
            0,
            Math.min(
              candidateLeft + cardWidth,
              obstacle.left + obstacle.width + clearance
            ) - Math.max(candidateLeft, obstacle.left - clearance)
          ) *
            Math.max(
              0,
              Math.min(
                candidateTop + cardHeight,
                obstacle.top + obstacle.height + clearance
              ) - Math.max(candidateTop, obstacle.top - clearance)
            ),
        0
      )
      const distance =
        (candidateLeft - preferred.left) ** 2 +
        (candidateTop - preferred.top) ** 2
      if (
        overlap < bestOverlap ||
        (overlap === bestOverlap && distance < bestDistance)
      ) {
        best = { left: candidateLeft, top: candidateTop }
        bestOverlap = overlap
        bestDistance = distance
      }
    }
  }
  return best
}
