/** Halve row capacity until balanced rows fit the measured available width. */
export function legendRowEnds(widths: readonly number[], available: number, gap: number): number[] {
  if (widths.length === 0) return []
  for (let capacity = widths.length; ; capacity = Math.ceil(capacity / 2)) {
    const rowCount = Math.ceil(widths.length / capacity)
    const smallRow = Math.floor(widths.length / rowCount)
    const largerRows = widths.length % rowCount
    const ends: number[] = []
    let start = 0
    let fits = true
    for (let row = 0; row < rowCount; row++) {
      const count = smallRow + (row < largerRows ? 1 : 0)
      const end = start + count
      const width = widths.slice(start, end).reduce((sum, value) => sum + value, 0)
        + (count - 1) * gap
      if (width > available) fits = false
      ends.push(end)
      start = end
    }
    if (fits || capacity === 1) return ends
  }
}
