// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { GRAPH_PALETTE_VERSION, GRAPH_SWATCHES } from '@/themes/graph-inks'
import { serviceInkSlots } from '@/themes/service-ink-slots'

beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
})

async function setup() {
  const slots = await import('./color-slots')
  const { accentInk } = await import('@/state/accent-ink.svelte')
  const { graphColorOverrides } =
    await import('@/state/graph-color-overrides.svelte')
  const { graphColorAvailability } = await import('./graph-color-availability')
  return { ...slots, accentInk, graphColorOverrides, graphColorAvailability }
}

for (const scope of ['metric:one', 'span-names']) {
  describe(`${scope} ink-based assignments`, () => {
    it('uses each ink ranking and restores exact owners across switches and reloads', async () => {
      const state = await setup()
      const names = ['', 'request', 'Request', 'request ', '服務', '__proto__']
      const assign = (name: string) => state.identityColorSlot(name, scope)
      const warm = names.map(assign)
      expect(warm).toEqual(serviceInkSlots('to-ro').slice(0, names.length))
      state.accentInk.set('ajisai')
      const cool = names.map(assign)
      expect(cool).toEqual(serviceInkSlots('ajisai').slice(0, names.length))
      expect(cool).not.toEqual(warm)
      assign('discovered while cool')
      state.accentInk.set('to-ro')
      expect([...names].reverse().map(assign)).toEqual([...warm].reverse())
      expect(state.savedColorSlots(scope).map(([name]) => name)).toContain(
        'discovered while cool'
      )
      vi.resetModules()
      const restored = await setup()
      expect(
        names.map(name => restored.identityColorSlot(name, scope))
      ).toEqual(warm)
      restored.accentInk.set('ajisai')
      expect(
        names.map(name => restored.identityColorSlot(name, scope))
      ).toEqual(cool)
      // A different metric has its own owners and starts at the same rank.
      expect(restored.identityColorSlot('independent', 'metric:other')).toBe(
        cool[0]
      )
    })

    it('keeps saved assignments on the initial ink and reserves hidden/manual owners across inks', async () => {
      const key = `graph-colour-slots:${GRAPH_PALETTE_VERSION}:224:${scope}`
      const saved = JSON.stringify([
        ['existing', 19],
        ['hidden', 21],
      ])
      localStorage.setItem(key, saved)
      const state = await setup()
      expect(state.identityColorSlot('existing', scope)).toBe(19)
      const manual = GRAPH_SWATCHES[serviceInkSlots('ajisai')[0]!]!
      state.graphColorOverrides.set(scope, 'manual owner', manual.id)
      state.accentInk.set('ajisai')
      const cool = state.identityColorSlot('existing', scope)
      expect(cool).not.toBe(serviceInkSlots('ajisai')[0])
      expect(state.graphColorAvailability(scope, 'new').unavailable).toContain(
        GRAPH_SWATCHES[21]!.color
      )
      expect(state.graphColorOverrides.get(scope, 'manual owner')?.color).toBe(
        manual.color
      )
      state.accentInk.set('to-ro')
      expect(state.identityColorSlot('existing', scope)).toBe(19)
      expect(state.identityColorSlot('hidden', scope)).toBe(21)
      expect(localStorage.getItem(key)).toBe(saved)
    })

    it('uses all 224 unique shades before reusing any slot', async () => {
      const state = await setup()
      const assigned = Array.from({ length: 224 }, (_, i) =>
        state.identityColorSlot(`owner-${i}`, scope)
      )
      expect(assigned).toEqual(serviceInkSlots('to-ro'))
      expect(new Set(assigned).size).toBe(224)
      expect(state.graphColorAvailability(scope, 'owner-0').freePresets).toBe(0)
      expect(state.colorSlotReuses(scope)).toEqual([])
    })
  })
}
