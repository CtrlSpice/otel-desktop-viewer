/** Fill each row in order, avoiding a lone final entry when the last pair fits. */
export function legendRowEnds(
  widths: readonly number[],
  available: number,
  gap: number
): number[] {
  if (widths.length === 0) return []
  const ends: number[] = []
  let start = 0
  let used = 0
  for (let index = 0; index < widths.length; index++) {
    const next = used + (index > start ? gap : 0) + widths[index]!
    if (index > start && next > available) {
      ends.push(index)
      start = index
      used = widths[index]!
    } else {
      used = next
    }
  }
  ends.push(widths.length)
  const previousEnd = ends.at(-2)
  const previousStart = ends.at(-3) ?? 0
  if (
    previousEnd === widths.length - 1 &&
    previousEnd - previousStart > 2 &&
    widths.at(-2)! + gap + widths.at(-1)! <= available
  ) {
    ends[ends.length - 2] = previousEnd - 1
  }
  return ends
}
