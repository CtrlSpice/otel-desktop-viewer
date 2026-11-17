import { expect, it } from 'vitest'
import { GRAPH_INKS, graphInkCurve } from '@/themes/graph-inks'
import { graphInkFamily } from './graph-ink-family'

it('recognizes all official shades and retains their seven-shade families', () => {
  for (const ink of GRAPH_INKS)
    for (const swatch of ink.swatches) {
      const result = graphInkFamily(swatch.color, swatch.id)
      expect(result.ink.id).toBe(ink.id)
      expect(result.exact).toBe(true)
      expect(result.ink.swatches).toHaveLength(7)
    }
})

it('retains named curve ownership and ignores opacity for nearest ink matching', () => {
  const curve = graphInkCurve('to-ro')[123]!
  expect(graphInkFamily(curve.color, curve.id).ink.id).toBe('to-ro')
  const custom = graphInkFamily('#123abc')
  expect(custom.exact).toBe(false)
  for (const alpha of ['00', '80', 'ff'])
    expect(graphInkFamily(`#123abc${alpha}`)).toEqual(custom)
  const official = GRAPH_INKS[0]!.swatches[2]!.color
  expect(graphInkFamily(`${official}80`).exact).toBe(true)
})
