// @vitest-environment jsdom
import { beforeEach, expect, it, vi } from 'vitest'
import {
  GRAPH_SWATCHES,
  GRAPH_PALETTE_VERSION,
  graphInkCurve,
} from '@/themes/graph-inks'
import { serviceInkSlots } from '@/themes/service-ink-slots'

beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
})

async function setup() {
  const { accentInk } = await import('@/state/accent-ink.svelte')
  const { graphColorOverrides } =
    await import('@/state/graph-color-overrides.svelte')
  const { traceServiceColor, spanNameColor } =
    await import('./trace-service-color')
  const { savedColorSlots, identityColorSlot } = await import('./color-slots')
  const { graphColorAvailability } = await import('./graph-color-availability')
  const color = (identity: string) =>
    traceServiceColor(identity, 'madoromi-aki-hiru')
  return {
    accentInk,
    graphColorOverrides,
    color,
    spanNameColor,
    savedColorSlots,
    identityColorSlot,
    graphColorAvailability,
  }
}

it('repaints automatic services by ink and restores exact assignments after switching back and reloading', async () => {
  const state = await setup()
  const identities = ['', 'api', 'API', 'api ', '__proto__', '服務']
  const warm = identities.map(state.color)
  expect(warm).toEqual(
    serviceInkSlots('to-ro')
      .slice(0, identities.length)
      .map(slot => GRAPH_SWATCHES[slot]!.color)
  )
  const series = state.identityColorSlot('api', 'metric:one')
  const span = state.spanNameColor('request', 'madoromi-aki-hiru')
  state.accentInk.set('ajisai')
  const cool = identities.map(state.color)
  expect(cool).not.toEqual(warm)
  state.color('added while cool')
  expect([...identities].reverse().map(state.color)).toEqual(
    [...cool].reverse()
  )
  state.accentInk.set('to-ro')
  expect(identities.map(state.color)).toEqual(warm)
  expect(state.savedColorSlots('services').map(([id]) => id)).toContain(
    'added while cool'
  )
  expect(state.identityColorSlot('api', 'metric:one')).toBe(series)
  expect(state.spanNameColor('request', 'madoromi-fuyu-yoru')).toBe(span)
  vi.resetModules()
  const restored = await setup()
  expect(identities.map(restored.color)).toEqual(warm)
  restored.accentInk.set('ajisai')
  expect(identities.map(restored.color)).toEqual(cool)
})

it('retains manual presets and curve choices, hidden reservations and per-ink reset slots', async () => {
  const state = await setup()
  const reserved = GRAPH_SWATCHES[serviceInkSlots('to-ro')[0]!]!
  state.graphColorOverrides.set('services', 'hidden manual', reserved.id)
  const curve = graphInkCurve('to-ro')[123]!
  state.graphColorOverrides.set('services', 'slider', curve.id)
  const automatic = state.color('api')
  expect(automatic).not.toBe(reserved.color)
  const manual = GRAPH_SWATCHES.find(
    swatch =>
      !state
        .graphColorAvailability('services', 'api')
        .unavailable.has(swatch.color) && swatch.color !== automatic
  )!
  state.graphColorOverrides.set('services', 'api', manual.id)
  state.accentInk.set('ajisai')
  expect(state.color('api')).toBe(manual.color)
  expect(state.color('slider')).toBe(curve.color)
  expect(state.color('new')).not.toBe(manual.color)
  expect(state.graphColorAvailability('services', 'new').unavailable).toContain(
    automatic
  )
  state.graphColorOverrides.set('services', 'api', null)
  const coolReset = state.color('api')
  expect(coolReset).not.toBe(manual.color)
  state.accentInk.set('to-ro')
  expect(state.color('api')).toBe(automatic)
  expect(state.color('hidden manual')).toBe(reserved.color)
  state.accentInk.set('ajisai')
  expect(state.color('api')).toBe(coolReset)
})

it('adopts existing saved slots for the initial ink without erasing their storage or moving hidden owners', async () => {
  const key = `graph-colour-slots:${GRAPH_PALETTE_VERSION}:224:services`
  const saved = JSON.stringify([
    ['api', 19],
    ['hidden', 21],
  ])
  localStorage.setItem(key, saved)
  localStorage.setItem('theme-accent-ink', 'ajisai')
  const state = await setup()
  expect(state.color('api')).toBe(GRAPH_SWATCHES[19]!.color)
  state.accentInk.set('to-ro')
  expect(state.color('api')).not.toBe(GRAPH_SWATCHES[19]!.color)
  expect(state.savedColorSlots('services').map(([id]) => id)).toContain(
    'hidden'
  )
  state.accentInk.set('ajisai')
  expect(state.color('hidden')).toBe(GRAPH_SWATCHES[21]!.color)
  expect(localStorage.getItem(key)).toBe(saved)
})

it('reaches every official swatch without reuse before exhaustion and retains slider fallback', async () => {
  const state = await setup()
  const colors = Array.from({ length: 224 }, (_, index) =>
    state.color(`service-${index}`)
  )
  expect(new Set(colors)).toEqual(
    new Set(GRAPH_SWATCHES.map(swatch => swatch.color))
  )
  const availability = state.graphColorAvailability('services', 'service-0')
  expect(availability.freePresets).toBe(0)
  expect(availability.unavailable.size).toBe(223)
  expect(availability.unavailable.has(colors[0]!)).toBe(false)
})

it('merges cross-tab hidden owners before preparing another ink', async () => {
  const state = await setup()
  const existing = state.color('existing')
  const key = `graph-colour-slots:${GRAPH_PALETTE_VERSION}:224:services:ink:to-ro`
  const hiddenSlot = serviceInkSlots('to-ro')[1]!
  const saved: [string, number][] = JSON.parse(localStorage.getItem(key)!)
  localStorage.setItem(
    key,
    JSON.stringify([...saved, ['hidden tab owner', hiddenSlot]])
  )
  window.dispatchEvent(new StorageEvent('storage', { key }))
  state.accentInk.set('ajisai')
  state.color('new')
  expect(state.savedColorSlots('services').map(([id]) => id)).toContain(
    'hidden tab owner'
  )
  state.accentInk.set('to-ro')
  expect(state.color('existing')).toBe(existing)
  expect(state.color('hidden tab owner')).toBe(
    GRAPH_SWATCHES[hiddenSlot]!.color
  )
  expect(state.color('new')).not.toBe(GRAPH_SWATCHES[hiddenSlot]!.color)
})

it('keeps ink-switch stability when browser storage is blocked', async () => {
  const state = await setup()
  const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('blocked')
  })
  try {
    const warm = state.color('service')
    state.accentInk.set('ajisai')
    const cool = state.color('service')
    expect(cool).not.toBe(warm)
    state.color('added')
    state.accentInk.set('to-ro')
    expect(state.color('service')).toBe(warm)
    state.accentInk.set('ajisai')
    expect(state.color('service')).toBe(cool)
  } finally {
    spy.mockRestore()
  }
})
