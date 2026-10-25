// @vitest-environment jsdom
import { beforeEach, expect, it, vi } from 'vitest'
import { GRAPH_PALETTE_VERSION } from '@/themes/graph-inks'
import { AUTOMATIC_COLOR_SLOTS } from '@/themes/service-ink-slots'

beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
})

for (const scope of ['services', 'metric:one', 'span-names']) {
  it.each(['varied-3-2-v1', 'neighbours-3-2-v1', 'shades-3-1-v1'])(
    `${scope}: carries %s owners into the fixed palette without losing old storage`,
    async version => {
      const key = `graph-colour-slots:${GRAPH_PALETTE_VERSION}:224:${scope}:ink:rikka:${version}`
      const saved = JSON.stringify([
        ['existing', 19],
        ['hidden', 21],
      ])
      localStorage.setItem(key, saved)
      localStorage.setItem('theme-accent-ink', 'rikka')
      const state = await import('./color-slots')
      const existing = state.identityColorSlot('existing', scope)
      const added = state.identityColorSlot('new', scope)
      const hidden = state.identityColorSlot('hidden', scope)
      for (const slot of [existing, added, hidden])
        expect(AUTOMATIC_COLOR_SLOTS).toContain(slot)
      expect(new Set([existing, added, hidden]).size).toBe(3)
      expect(localStorage.getItem(key)).toBe(saved)
      vi.resetModules()
      localStorage.setItem('theme-accent-ink', 'to-ro')
      const restored = await import('./color-slots')
      expect(restored.identityColorSlot('existing', scope)).toBe(existing)
      expect(restored.identityColorSlot('new', scope)).toBe(added)
    }
  )
}
