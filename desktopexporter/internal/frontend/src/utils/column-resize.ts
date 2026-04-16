export type ColumnDef = {
  id: string
  min: number
  flex: number
}

export function fixed(id: string, width: number): ColumnDef {
  return { id, min: width, flex: 0 }
}

export function flex(id: string, min: number, weight: number): ColumnDef {
  return { id, min, flex: weight }
}

/**
 * Compute initial pixel widths from column defs and container width.
 * Fixed columns get their min. Flex columns share remaining space by weight.
 */
export function computeInitialWidths(
  defs: ColumnDef[],
  containerWidth: number
): number[] {
  const fixedTotal = defs.reduce((s, d) => (d.flex === 0 ? s + d.min : s), 0)
  const flexTotal = defs.reduce((s, d) => s + d.flex, 0)
  const available = Math.max(0, containerWidth - fixedTotal)

  return defs.map(d => {
    if (d.flex === 0) return d.min
    const proportional = flexTotal > 0 ? (d.flex / flexTotal) * available : 0
    return Math.max(d.min, proportional)
  })
}

/**
 * Redistribute widths when the container resizes.
 * The delta (new - old container width) is spread across flex columns
 * proportionally to their current widths, respecting minimums.
 */
export function redistributeWidths(
  defs: ColumnDef[],
  currentWidths: number[],
  newContainerWidth: number
): number[] {
  const currentTotal = currentWidths.reduce((a, b) => a + b, 0)
  const delta = newContainerWidth - currentTotal
  if (Math.abs(delta) < 1) return currentWidths

  const flexIndices: number[] = []
  let flexTotal = 0
  for (let i = 0; i < defs.length; i++) {
    if (defs[i].flex > 0) {
      flexIndices.push(i)
      flexTotal += currentWidths[i]
    }
  }
  if (flexIndices.length === 0 || flexTotal === 0) return currentWidths

  const next = [...currentWidths]
  let remaining = delta
  for (const i of flexIndices) {
    const share = (currentWidths[i] / flexTotal) * delta
    const newW = Math.max(defs[i].min, currentWidths[i] + share)
    const actual = newW - currentWidths[i]
    next[i] = newW
    remaining -= actual
  }
  return next
}

/**
 * Compute left-px position for each flex column's drag bar.
 * Only places a bar if there's a flex neighbor to the right (adjacent-column).
 */
export function computeBarPositions(
  defs: ColumnDef[],
  widths: number[]
): { index: number; left: number }[] {
  const result: { index: number; left: number }[] = []
  let cumulative = 0
  for (let i = 0; i < defs.length; i++) {
    cumulative += widths[i]
    if (defs[i].flex > 0) {
      const hasFlexRight = defs.slice(i + 1).some(d => d.flex > 0)
      if (hasFlexRight) {
        result.push({ index: i, left: cumulative })
      }
    }
  }
  return result
}

/**
 * Adjacent-column resize: the bar between two columns affects only that pair.
 * Dragging right grows colIndex and shrinks its next flex neighbor.
 * Dragging left does the reverse. Both are clamped at their min.
 */
export function applyColumnResize(
  defs: ColumnDef[],
  currentWidths: number[],
  colIndex: number,
  desiredWidth: number
): number[] {
  const def = defs[colIndex]
  if (!def || def.flex === 0) return currentWidths

  let neighborIdx = -1
  for (let i = colIndex + 1; i < defs.length; i++) {
    if (defs[i].flex > 0) { neighborIdx = i; break }
  }
  if (neighborIdx === -1) return currentWidths

  const neighbor = defs[neighborIdx]
  const maxShrink = currentWidths[colIndex] - def.min
  const maxGrow = currentWidths[neighborIdx] - neighbor.min

  const delta = Math.max(-maxShrink, Math.min(maxGrow, desiredWidth - currentWidths[colIndex]))
  if (delta === 0) return currentWidths

  const next = [...currentWidths]
  next[colIndex] += delta
  next[neighborIdx] -= delta
  return next
}

/**
 * Attach pointer-based column resize to a drag handle element.
 * Calls `onResize(newWidths)` on each pointer move.
 */
export function startColumnResize(
  defs: ColumnDef[],
  currentWidths: () => number[],
  colIndex: number,
  e: PointerEvent,
  onResize: (widths: number[]) => void,
  onEnd: () => void
) {
  e.preventDefault()
  const startX = e.clientX
  const startW = currentWidths()[colIndex]
  const target = e.currentTarget as HTMLElement
  target.setPointerCapture(e.pointerId)

  function onMove(ev: PointerEvent) {
    const desired = startW + (ev.clientX - startX)
    const next = applyColumnResize(defs, currentWidths(), colIndex, desired)
    if (next !== currentWidths()) onResize(next)
  }

  function end() {
    onEnd()
    target.removeEventListener('pointermove', onMove)
    target.removeEventListener('pointerup', end)
    target.removeEventListener('pointercancel', end)
  }

  target.addEventListener('pointermove', onMove)
  target.addEventListener('pointerup', end)
  target.addEventListener('pointercancel', end)
}
