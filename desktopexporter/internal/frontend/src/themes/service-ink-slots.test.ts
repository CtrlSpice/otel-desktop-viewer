import { describe, expect, it } from 'vitest'
import { ACCENT_INKS } from './accent-inks'
import accentFamilies from './accent-inks.json' with { type: 'json' }
import harmonies from './service-ink-harmonies.json' with { type: 'json' }
import { GRAPH_SWATCHES } from './graph-inks'
import { inkPreviewColors, serviceInkSlots } from './service-ink-slots'
import rankings from './bright-ink-slots.json'
import { ramps } from '../../../../../third_party/madoromi/ink-ramps.json'

describe('Madoromi Mix service ranking', () => {
  for (const { id } of ACCENT_INKS) {
    it(`${id}: includes all three preview shades and one triadic accent in its opening`, () => {
      const family = harmonies.inks[id]
      expect(family.analogous).toEqual(accentFamilies.inks[id].family)
      const ranked = serviceInkSlots(id).map(slot => GRAPH_SWATCHES[slot]!)
      expect(ranked[0]!.ink).toBe(id)
      const opening = ranked.slice(0, 4)
      expect(opening.filter(s => s.ink === id)).toHaveLength(3)
      expect(
        new Set(opening.filter(s => s.ink === id).map(s => s.color))
      ).toEqual(new Set(inkPreviewColors(id)))
      const contrast = opening.filter(s => s.ink !== id)
      expect(contrast).toHaveLength(1)
      expect(family.triadic).toContain(contrast[0]!.ink)
      const previewIDs = new Set<string>(
        Object.values(rankings.previews).flat()
      )
      const eligible = GRAPH_SWATCHES.filter(s => {
        const [L, a, b] = ramps[s.ink][s.shade! - 1]!
        return (
          previewIDs.has(s.id) ||
          (L! >= 0.6 && L! <= 0.87 && Math.hypot(a!, b!) >= 0.1)
        )
      })
      for (const s of opening.filter(s => s.ink === id)) {
        const [L] = ramps[s.ink][s.shade! - 1]!
        expect(L).toBeGreaterThanOrEqual(0.45)
        expect(L).toBeLessThanOrEqual(0.87)
      }
      expect(ranked).toHaveLength(eligible.length)
      expect(new Set(ranked.map(swatch => swatch.id))).toEqual(
        new Set(eligible.map(swatch => swatch.id))
      )
    })
  }
  it('gives Rikka purple, cyan and sky shades with an orange accent', () => {
    expect(
      serviceInkSlots('rikka')
        .slice(0, 4)
        .map(slot => GRAPH_SWATCHES[slot]!.id)
    ).toEqual(['rikka:6', 'fuyu-gaki:2', 'rikka:7', 'rikka:3'])
  })
})
