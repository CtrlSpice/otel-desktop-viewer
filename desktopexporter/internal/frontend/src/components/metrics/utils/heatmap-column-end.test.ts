import { describe, it, expect } from 'vitest'
import { heatmapColumnEndNs } from './heatmap-column-selection'

const S = 1_000_000_000n
const HOUR = 3600n * S
const WINDOW_END = 999_999_999_999_999n

describe('heatmapColumnEndNs', () => {
  it('ends one nanosecond before the next column', () => {
    const starts = [0n, 30n * S, 60n * S]
    expect(heatmapColumnEndNs(starts, 30n * S, WINDOW_END)).toBe(60n * S - 1n)
  })

  // Columns are cut in local time, so a local day is 23, 24 or 25 hours of real
  // time across a DST transition. Deriving one width and adding it -- the
  // smallest gap being the obvious choice -- would fetch 23 hours of the
  // 25-hour column and lose the other two, silently, which is the same
  // partial-column defect this whole fetch exists to remove.
  it('follows uneven columns across a DST transition', () => {
    const spring = 0n // 23h day
    const normal = 23n * HOUR // 24h day
    const autumn = 47n * HOUR // 25h day
    const after = 72n * HOUR
    const starts = [spring, normal, autumn, after]

    expect(heatmapColumnEndNs(starts, spring, WINDOW_END)).toBe(normal - 1n)
    expect(heatmapColumnEndNs(starts, normal, WINDOW_END)).toBe(autumn - 1n)
    // The 25-hour one: a minimum-gap width would have ended it 2 hours early.
    expect(heatmapColumnEndNs(starts, autumn, WINDOW_END)).toBe(after - 1n)
    expect(heatmapColumnEndNs(starts, autumn, WINDOW_END)).not.toBe(
      autumn + 23n * HOUR - 1n
    )
  })

  it('borrows the preceding gap for the final column', () => {
    const starts = [0n, 30n * S, 60n * S]
    expect(heatmapColumnEndNs(starts, 60n * S, WINDOW_END)).toBe(90n * S - 1n)
  })

  // A window that reduces to one column still renders it and still accepts a
  // click. Returning nothing here left the panel empty forever, with no error
  // and no way for the reader to tell it had failed.
  it('falls back to the window end for a lone column', () => {
    expect(heatmapColumnEndNs([100n * S], 100n * S, WINDOW_END)).toBe(
      WINDOW_END
    )
  })

  it('is null when even the window cannot bound the column', () => {
    expect(heatmapColumnEndNs([100n * S], 100n * S, 50n * S)).toBeNull()
  })

  it('handles an unsorted, duplicated column list', () => {
    const starts = [60n * S, 0n, 30n * S, 30n * S]
    expect(heatmapColumnEndNs(starts, 0n, WINDOW_END)).toBe(30n * S - 1n)
  })
})
