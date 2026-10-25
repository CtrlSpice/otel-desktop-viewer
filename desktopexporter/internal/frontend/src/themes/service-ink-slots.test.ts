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
    it(`${id}: separates its three preview shades with two distinct companion inks`, () => {
      const family = harmonies.inks[id]
      expect(family.analogous).toEqual(accentFamilies.inks[id].family)
      const ranked = serviceInkSlots(id).map(slot => GRAPH_SWATCHES[slot]!)
      expect(ranked[0]!.ink).toBe(id)
      const opening = ranked.slice(0, 5)
      expect(opening.filter(s => s.ink === id)).toHaveLength(3)
      expect(
        new Set(opening.filter(s => s.ink === id).map(s => s.color))
      ).toEqual(new Set(inkPreviewColors(id)))
      const contrast = opening.filter(s => s.ink !== id)
      expect(contrast).toHaveLength(2)
      expect(new Set(contrast.map(s => s.id))).toEqual(
        new Set(rankings.companions[id])
      )
      const [seedL, seedA, seedB] = ramps[id][opening[0]!.shade! - 1]!
      const seedHue = Math.atan2(seedB!, seedA!)
      for (const s of contrast) {
        const [L, a, b] = ramps[s.ink][s.shade! - 1]!
        const delta = Math.atan2(b!, a!) - seedHue
        const angle = Math.abs(Math.atan2(Math.sin(delta), Math.cos(delta)))
        expect(angle).toBeGreaterThanOrEqual(Math.PI / 4)
        expect(angle).toBeLessThanOrEqual((100 * Math.PI) / 180)
        expect(Math.abs(L! - seedL!)).toBeLessThanOrEqual(0.18)
        for (const other of opening.filter(other => other !== s)) {
          const lab = ramps[other.ink][other.shade! - 1]!
          expect(
            Math.hypot(L! - lab[0]!, a! - lab[1]!, b! - lab[2]!)
          ).toBeGreaterThanOrEqual(0.1)
        }
      }
      // Side colours are interspersed, not appended together after the seed shades.
      const positions = contrast.map(s => opening.indexOf(s))
      expect(Math.abs(positions[0]! - positions[1]!)).toBeGreaterThan(1)
      for (let i = 1; i < opening.length; i++)
        expect(opening[i]!.ink).not.toBe(opening[i - 1]!.ink)
      expect(
        new Set(ranked.slice(0, 10).map(s => s.ink)).size
      ).toBeGreaterThanOrEqual(6)
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
})
