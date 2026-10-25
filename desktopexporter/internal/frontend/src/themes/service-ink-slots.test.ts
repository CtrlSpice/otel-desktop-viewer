import { expect, it } from 'vitest'
import { GRAPH_INKS, GRAPH_SWATCHES } from './graph-inks'
import { AUTOMATIC_COLOR_SLOTS } from './service-ink-slots'
import { ramps } from '../../../../../third_party/madoromi/ink-ramps.json'

it('uses every bright official shade and every ink family before extra shades', () => {
  const ranked = AUTOMATIC_COLOR_SLOTS.map(slot => GRAPH_SWATCHES[slot]!)
  const eligible = GRAPH_SWATCHES.filter(s => {
    const [L, a, b] = ramps[s.ink][s.shade! - 1]!
    return L! >= 0.55 && L! <= 0.87 && Math.hypot(a!, b!) >= 0.1
  })
  expect(new Set(ranked.map(s => s.id))).toEqual(
    new Set(eligible.map(s => s.id))
  )
  expect(ranked).toHaveLength(eligible.length)
  expect(new Set(ranked.slice(0, 32).map(s => s.ink))).toEqual(
    new Set(GRAPH_INKS.map(ink => ink.id))
  )
})
