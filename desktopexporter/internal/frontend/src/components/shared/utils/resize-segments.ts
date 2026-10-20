export type RowBounds = { top: number; bottom: number }

/** Visible resize regions, excluding spanning rows but retaining the sticky header. */
export function resizeSegments(
  height: number,
  headerHeight: number,
  scrollTop: number,
  excluded: readonly RowBounds[]
): RowBounds[] {
  const header = Math.min(height, headerHeight)
  const segments: RowBounds[] = []
  let top = 0
  for (const row of excluded) {
    const start = Math.min(height, Math.max(header, row.top - scrollTop))
    const end = Math.min(height, Math.max(header, row.bottom - scrollTop))
    if (start > top) segments.push({ top, bottom: start })
    top = Math.max(top, end)
  }
  if (top < height) segments.push({ top, bottom: height })
  return segments
}
