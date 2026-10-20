import { describe, expect, it } from 'vitest'
import { legendRowEnds } from './legend-rows'

describe('legendRowEnds', () => {
  const widths = Array<number>(12).fill(60)

  it('fills available width without leaving a lone final entry', () => {
    expect(legendRowEnds(widths, 896, 16)).toEqual([12])
    expect(legendRowEnds(widths, 895, 16)).toEqual([10, 12])
    expect(legendRowEnds(widths, 668, 16)).toEqual([9, 12])
    expect(legendRowEnds(widths, 440, 16)).toEqual([6, 12])
    expect(legendRowEnds(widths, 439, 16)).toEqual([5, 10, 12])
  })

  it('moves one item to the final row only when needed to avoid a lone item', () => {
    expect(legendRowEnds(widths.slice(0, 9), 364, 16)).toEqual([5, 9])
    expect(legendRowEnds(widths.slice(0, 10), 212, 16)).toEqual([3, 6, 8, 10])
  })

  it('measures each whole row, including wider count labels and gaps', () => {
    expect(legendRowEnds([40, 40, 100, 100], 215, 16)).toEqual([3, 4])
    expect(legendRowEnds([40, 40, 100, 100], 216, 16)).toEqual([2, 4])
  })

  it('uses single-entry rows when pairs cannot fit and retains empty/single legends', () => {
    expect(legendRowEnds(widths, 60, 16)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12])
    expect(legendRowEnds([], 100, 16)).toEqual([])
    expect(legendRowEnds([100], 80, 16)).toEqual([1])
  })
})
