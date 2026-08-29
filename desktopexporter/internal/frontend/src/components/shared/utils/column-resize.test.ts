import { describe, expect, it } from 'vitest'

import {
  fixed,
  flex,
  initialWidths,
  fitWidths,
  reconcileWidths,
  resizeBar,
  barPositions,
  type ColumnWidths,
} from './column-resize'

/* These tests are the cascade's specification, written before the
 * implementation. The column set is about to become user-configurable,
 * so everything here works in ids, never positions, and the add/remove
 * cases are part of the spec from day one. */

const abc = [flex('a', 100, 1), flex('b', 100, 1), flex('c', 100, 1)]

function total(w: ColumnWidths): number {
  return Object.values(w).reduce((s, x) => s + x, 0)
}

describe('initial layout', () => {
  it('gives fixed columns their width and shares the rest by weight', () => {
    const specs = [fixed('gutter', 40), flex('a', 100, 1), flex('b', 100, 3)]
    const w = initialWidths(specs, 440)
    expect(w).toEqual({ gutter: 40, a: 100, b: 300 })
  })

  it('floors flex columns at their min even when the container is tight', () => {
    const w = initialWidths(abc, 150)
    expect(w.a).toBeGreaterThanOrEqual(100)
    expect(w.b).toBeGreaterThanOrEqual(100)
    expect(w.c).toBeGreaterThanOrEqual(100)
  })
})

describe('cascade resize', () => {
  // Bars are named by the column they follow; dragging right grows that
  // column, taking space from the columns beyond the bar.
  it('takes from the immediate neighbour while it has room', () => {
    const w = resizeBar(abc, initialWidths(abc, 600), 'a', 50)
    expect(w).toEqual({ a: 250, b: 150, c: 200 })
  })

  it('cascades to the next column when the neighbour hits its min', () => {
    // b can only give 100; the remaining 50 comes from c.
    const w = resizeBar(abc, initialWidths(abc, 600), 'a', 150)
    expect(w).toEqual({ a: 350, b: 100, c: 150 })
  })

  it('stops the handle when every column beyond it is at min', () => {
    const start = { a: 400, b: 100, c: 100 }
    expect(resizeBar(abc, start, 'a', 60)).toEqual(start)
  })

  it('clamps a drag that wants more than the cascade can supply', () => {
    // 500 requested, only 200 available beyond the bar.
    const w = resizeBar(abc, initialWidths(abc, 600), 'a', 500)
    expect(w).toEqual({ a: 400, b: 100, c: 100 })
  })

  it('is symmetric dragging left, cascading through columns before the bar', () => {
    // Bar after b, dragged 150 left: b gives its 100, a gives 50, and the
    // nearest column beyond the bar receives all of it.
    const w = resizeBar(abc, initialWidths(abc, 600), 'b', -150)
    expect(w).toEqual({ a: 150, b: 100, c: 350 })
  })

  it('never moves a fixed column, in either direction', () => {
    const specs = [flex('a', 100, 1), fixed('gutter', 40), flex('b', 100, 1)]
    const start = { a: 200, gutter: 40, b: 200 }
    const right = resizeBar(specs, start, 'a', 80)
    expect(right.gutter).toBe(40)
    expect(right).toEqual({ a: 280, gutter: 40, b: 120 })
    const left = resizeBar(specs, start, 'a', -80)
    expect(left.gutter).toBe(40)
    expect(left).toEqual({ a: 120, gutter: 40, b: 280 })
  })

  it('conserves total width under every drag', () => {
    const start = initialWidths(abc, 600)
    for (const delta of [-500, -150, -1, 0, 1, 50, 150, 500]) {
      for (const bar of ['a', 'b']) {
        expect(total(resizeBar(abc, start, bar, delta))).toBeCloseTo(600, 6)
      }
    }
  })

  it('returns the input untouched for an unknown or fixed bar id', () => {
    const start = initialWidths(abc, 600)
    expect(resizeBar(abc, start, 'nope', 50)).toEqual(start)
    const specs = [fixed('gutter', 40), flex('a', 100, 1), flex('b', 100, 1)]
    const w = initialWidths(specs, 440)
    expect(resizeBar(specs, w, 'gutter', 50)).toEqual(w)
  })
})

describe('container refit', () => {
  it('spreads growth across flex columns in proportion to their widths', () => {
    const w = fitWidths(abc, { a: 300, b: 200, c: 100 }, 900)
    expect(total(w)).toBeCloseTo(900, 6)
    // a had half the flex width, so it gets half the growth.
    expect(w.a).toBeCloseTo(450, 6)
  })

  it('respects minimums when the container shrinks', () => {
    const w = fitWidths(abc, { a: 300, b: 200, c: 100 }, 350)
    expect(w.c).toBeGreaterThanOrEqual(100)
    expect(w.b).toBeGreaterThanOrEqual(100)
  })

  it('leaves fixed columns alone', () => {
    const specs = [fixed('gutter', 40), flex('a', 100, 1)]
    const w = fitWidths(specs, { gutter: 40, a: 200 }, 640)
    expect(w.gutter).toBe(40)
  })
})

describe('reconcile against a stored layout', () => {
  // The operation a column setting calls on every add/remove, and the
  // same one that runs on load against whatever an older session stored.
  it('keeps a surviving id at its stored width', () => {
    const w = reconcileWidths(abc, { a: 300, b: 150, c: 150 }, 600)
    expect(w).toEqual({ a: 300, b: 150, c: 150 })
  })

  it('drops a stale id that no longer has a column', () => {
    const w = reconcileWidths(abc, { a: 300, b: 150, ghost: 500 }, 600)
    expect(w.ghost).toBeUndefined()
    expect(Object.keys(w).sort()).toEqual(['a', 'b', 'c'])
  })

  it('gives a new id its share and renormalizes to the container', () => {
    // Stored from a two-column world; c is new.
    const w = reconcileWidths(abc, { a: 300, b: 300 }, 600)
    expect(w.c).toBeGreaterThanOrEqual(100)
    expect(total(w)).toBeCloseTo(600, 6)
  })

  it('clamps a stored width below the column minimum', () => {
    const w = reconcileWidths(abc, { a: 20, b: 290, c: 290 }, 600)
    expect(w.a).toBeGreaterThanOrEqual(100)
    expect(total(w)).toBeCloseTo(600, 6)
  })

  it('is a fresh layout when nothing was stored', () => {
    expect(reconcileWidths(abc, {}, 600)).toEqual(initialWidths(abc, 600))
  })
})

describe('bar placement', () => {
  it('puts a bar after each flex column that has a flex column beyond it', () => {
    const w = initialWidths(abc, 600)
    expect(barPositions(abc, w)).toEqual([
      { id: 'a', left: 200, min: 100, max: 400 },
      { id: 'b', left: 400, min: 200, max: 500 },
    ])
  })

  it('places no bar when only one flex column exists', () => {
    const specs = [fixed('gutter', 40), flex('a', 100, 1)]
    expect(barPositions(specs, initialWidths(specs, 640))).toEqual([])
  })

  it('skips fixed columns when deciding what counts as beyond', () => {
    const specs = [flex('a', 100, 1), fixed('gutter', 40), flex('b', 100, 1)]
    const w = initialWidths(specs, 440)
    // The bar after a exists because b lies beyond, and its px position
    // includes the fixed gutter it sits before.
    expect(barPositions(specs, w)).toEqual([
      { id: 'a', left: w.a, min: 100, max: 300 },
    ])
  })

  it('reports ranges from the same cascade slack used by resizing', () => {
    const widths = { a: 350, b: 100, c: 150 }
    expect(barPositions(abc, widths)).toEqual([
      { id: 'a', left: 350, min: 100, max: 400 },
      { id: 'b', left: 450, min: 200, max: 500 },
    ])
  })
})
