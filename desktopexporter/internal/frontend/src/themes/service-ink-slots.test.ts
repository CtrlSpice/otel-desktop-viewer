import { describe, expect, it } from 'vitest'
import { ACCENT_INKS } from './accent-inks'
import accentFamilies from './accent-inks.json' with { type: 'json' }
import harmonies from './service-ink-harmonies.json' with { type: 'json' }
import { GRAPH_SWATCHES } from './graph-inks'
import { serviceInkSlots } from './service-ink-slots'
import { ramps } from '../../../../../third_party/madoromi/ink-ramps.json'

describe('Madoromi Mix service ranking', () => {
  for (const { id } of ACCENT_INKS) {
    it(`${id}: starts with three related and two triadic bright colours`, () => {
      const family = harmonies.inks[id]
      expect(family.analogous).toEqual(accentFamilies.inks[id].family)
      const ranked = serviceInkSlots(id).map(slot => GRAPH_SWATCHES[slot]!)
      expect(ranked[0]!.ink).toBe(id)
      const opening = ranked.slice(0, 5)
      expect(
        opening.filter(s => family.analogous.includes(s.ink))
      ).toHaveLength(3)
      expect(
        opening
          .filter(s => !family.analogous.includes(s.ink))
          .map(s => s.ink)
          .sort()
      ).toEqual(family.triadic.filter(ink => ink !== id).sort())
      const eligible = GRAPH_SWATCHES.filter(s => {
        const [L, a, b] = ramps[s.ink][s.shade! - 1]!
        return L! >= 0.6 && L! <= 0.87 && Math.hypot(a!, b!) >= 0.1
      })
      expect(ranked).toHaveLength(eligible.length)
      expect(new Set(ranked.map(swatch => swatch.id))).toEqual(
        new Set(eligible.map(swatch => swatch.id))
      )
    })
  }
  it('retains the approved Rikka opening', () => {
    expect(
      serviceInkSlots('rikka')
        .slice(0, 5)
        .map(slot => GRAPH_SWATCHES[slot]!.id)
    ).toEqual([
      'rikka:6',
      'fuyu-gaki:2',
      'ku-jaku:6',
      'murasaki-shikibu:4',
      'shin-ryoku:7',
    ])
  })
})
