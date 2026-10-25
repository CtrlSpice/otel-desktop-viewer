import { describe, expect, it } from 'vitest'
import { ACCENT_INKS } from './accent-inks'
import accentFamilies from './accent-inks.json' with { type: 'json' }
import harmonies from './service-ink-harmonies.json' with { type: 'json' }
import { GRAPH_SWATCHES } from './graph-inks'
import { serviceInkSlots } from './service-ink-slots'

describe('Madoromi Mix service ranking', () => {
  for (const { id } of ACCENT_INKS) {
    it(`${id}: starts with the upstream analogous family and an early complementary accent`, () => {
      const family = harmonies.inks[id]
      expect(family.analogous).toEqual(accentFamilies.inks[id].family)
      const core = [...new Set([...family.analogous, ...family.complementary])]
      const ranked = serviceInkSlots(id).map(slot => GRAPH_SWATCHES[slot]!)
      expect(ranked.slice(0, core.length).map(swatch => swatch.ink)).toEqual(
        core
      )
      expect(core.length).toBeGreaterThan(family.analogous.length)
      expect(
        ranked.slice(0, core.length).every(swatch => swatch.shade === 4)
      ).toBe(true)
      expect(
        new Set(ranked.slice(0, core.length).map(swatch => swatch.ink)).size
      ).toBe(core.length)
      expect(
        ranked
          .slice(0, core.length * 3)
          .every(swatch => core.includes(swatch.ink))
      ).toBe(true)
      expect(ranked).toHaveLength(224)
      expect(new Set(ranked.map(swatch => swatch.id))).toEqual(
        new Set(GRAPH_SWATCHES.map(swatch => swatch.id))
      )
      expect(new Set(ranked.map(swatch => swatch.color))).toEqual(
        new Set(GRAPH_SWATCHES.map(swatch => swatch.color))
      )
    })
  }
})
