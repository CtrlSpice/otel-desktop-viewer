import { expect, it } from 'vitest'
import { buildStickyTree, stickyAncestors } from './waterfall-sticky'

const tree = (depths: number[]) =>
  buildStickyTree(depths.map(depth => ({ depth })))
const indices = (depths: number[], top: number, height = 40) =>
  stickyAncestors(tree(depths), top, height).map(row => row.index)

it('pins the offscreen ancestry of the current branch, including consecutive parent rows', () => {
  const depths = [0, 1, 2, 2, 1, 2, 0, 1]
  expect(indices(depths, 0)).toEqual([])
  expect(indices(depths, 1)).toEqual([0, 1])
  expect(indices(depths, 41)).toEqual([0, 1])
  expect(indices(depths, 120)).toEqual([0])
  expect(indices(depths, 121)).toEqual([0, 4])
  expect(indices(depths, 240)).toEqual([])
  expect(indices(depths, 241)).toEqual([6])
})
it('pushes ancestors out as the next branch reaches their position', () => {
  const value = tree([0, 1, 2, 2, 1, 2, 0, 1])
  expect(stickyAncestors(value, 81, 40)).toEqual([
    { index: 0, offset: 0 },
    { index: 1, offset: -1 },
  ])
  expect(stickyAncestors(value, 201, 40)).toEqual([{ index: 0, offset: -1 }])
  expect(stickyAncestors(value, 239, 40)).toEqual([{ index: 0, offset: -39 }])
})
it('uses only the rendered hierarchy after collapse, filtering or subtree focus', () => {
  expect(indices([0, 1, 1, 1], 41)).toEqual([0])
  expect(indices([0, 1, 2], 41)).toEqual([0, 1])
  expect(indices([0, 0, 1], 41)).toEqual([1])
  expect(indices([0, 3, 4, 4], 81)).toEqual([1])
})
it('adapts to measured row height and retains deep ancestors in a scrollable context area', () => {
  expect(indices([0, 1, 2, 2, 1, 2], 51, 50)).toEqual([0, 1])
  const value = tree(Array.from({ length: 1000 }, (_, index) => index))
  const result = stickyAncestors(value, 4000, 40, 80)
  expect(result.map(row => row.index)).toEqual(
    Array.from({ length: 102 }, (_, index) => index)
  )
})
it.each([0, -1, Infinity, NaN, 1000])(
  'does not produce stale ancestors outside the scroll range: %s',
  top => {
    expect(stickyAncestors(tree([0, 1]), top, 40)).toEqual([])
  }
)
