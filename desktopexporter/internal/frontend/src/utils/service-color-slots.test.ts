// @vitest-environment jsdom
import { beforeEach, expect, it, vi } from 'vitest'
import {
  GRAPH_SWATCHES,
  GRAPH_PALETTE_VERSION,
  graphInkCurve,
} from '@/themes/graph-inks'
import {
  AUTOMATIC_COLOR_VERSION,
  AUTOMATIC_COLOR_SLOTS,
} from '@/themes/service-ink-slots'

beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
})

async function setup() {
  const { graphColorOverrides } =
    await import('@/state/graph-color-overrides.svelte')
  const { traceServiceColor, spanNameColor } =
    await import('./trace-service-color')
  const { savedColorSlots, identityColorSlot } = await import('./color-slots')
  const color = (identity: string) =>
    traceServiceColor(identity, 'madoromi-aki-hiru')
  return {
    graphColorOverrides,
    color,
    spanNameColor,
    savedColorSlots,
    identityColorSlot,
  }
}

it('keeps exact service colours across seasons, reordering and reloads', async () => {
  const state = await setup()
  const identities = ['', 'api', 'API', 'api ', '__proto__', '服務']
  const warm = identities.map(state.color)
  const pool = AUTOMATIC_COLOR_SLOTS.map(slot => GRAPH_SWATCHES[slot]!.color)
  for (const color of warm) expect(pool).toContain(color)
  expect(new Set(warm).size).toBe(identities.length)
  const series = state.identityColorSlot('api', 'metric:one')
  const span = state.spanNameColor('request', 'madoromi-aki-hiru')
  const cool = identities.map(state.color)
  expect(cool).toEqual(warm)
  state.color('added while cool')
  expect([...identities].reverse().map(state.color)).toEqual(
    [...cool].reverse()
  )
  expect(identities.map(state.color)).toEqual(warm)
  expect(state.savedColorSlots('services').map(([id]) => id)).toContain(
    'added while cool'
  )
  expect(state.identityColorSlot('api', 'metric:one')).toBe(series)
  expect(state.spanNameColor('request', 'madoromi-fuyu-yoru')).toBe(span)
  vi.resetModules()
  const restored = await setup()
  expect(identities.map(restored.color)).toEqual(warm)
  expect(identities.map(restored.color)).toEqual(cool)
})

it('retains saved manual presets, curve choices and hidden reservations', async () => {
  const state = await setup()
  const reserved = GRAPH_SWATCHES[AUTOMATIC_COLOR_SLOTS[0]!]!
  state.graphColorOverrides.set('services', 'hidden manual', reserved.id)
  const curve = graphInkCurve('to-ro')[123]!
  state.graphColorOverrides.set('services', 'slider', curve.id)
  const automatic = state.color('api')
  expect(automatic).not.toBe(reserved.color)
  const manual = GRAPH_SWATCHES.find(
    swatch => ![reserved.color, curve.color, automatic].includes(swatch.color)
  )!
  state.graphColorOverrides.set('services', 'api', manual.id)
  expect(state.color('api')).toBe(manual.color)
  expect(state.color('slider')).toBe(curve.color)
  expect(state.color('new')).not.toBe(manual.color)
  expect(state.color('new')).not.toBe(automatic)
  state.graphColorOverrides.set('services', 'api', null)
  const coolReset = state.color('api')
  expect(coolReset).not.toBe(manual.color)
  expect(state.color('api')).toBe(automatic)
  expect(state.color('hidden manual')).toBe(reserved.color)
  expect(state.color('api')).toBe(coolReset)
})

it('recolours old automatic slots while retaining hidden owners and old storage', async () => {
  const key = `graph-colour-slots:${GRAPH_PALETTE_VERSION}:224:services`
  const saved = JSON.stringify([
    ['api', 19],
    ['hidden', 21],
  ])
  localStorage.setItem(key, saved)
  localStorage.setItem('theme-accent-ink', 'ajisai')
  const state = await setup()
  const opening = AUTOMATIC_COLOR_SLOTS.map(slot => GRAPH_SWATCHES[slot]!.color)
  expect(opening).toContain(state.color('api'))
  expect(state.color('api')).not.toBe(GRAPH_SWATCHES[19]!.color)
  expect(state.savedColorSlots('services').map(([id]) => id)).toContain(
    'hidden'
  )
  expect(opening).toContain(state.color('hidden'))
  expect(state.color('hidden')).not.toBe(state.color('api'))
  expect(localStorage.getItem(key)).toBe(saved)
})

it('exhausts only the bright pool, then reuses it without moving owners', async () => {
  const state = await setup()
  const preferred = AUTOMATIC_COLOR_SLOTS
  const colors = Array.from({ length: preferred.length }, (_, index) =>
    state.color(`service-${index}`)
  )
  expect(new Set(colors)).toEqual(
    new Set(preferred.map(slot => GRAPH_SWATCHES[slot]!.color))
  )
  const warning = vi.spyOn(console, 'warn').mockImplementation(() => {})
  try {
    for (let i = 0; i < 50; i++)
      expect(colors).toContain(state.color(`overflow-${i}`))
    expect(colors.map((_, i) => state.color(`service-${i}`))).toEqual(colors)
  } finally {
    warning.mockRestore()
  }
})

it('merges cross-tab hidden owners before assigning a new service', async () => {
  const state = await setup()
  const existing = state.color('existing')
  const key = `graph-colour-slots:${GRAPH_PALETTE_VERSION}:224:services:${AUTOMATIC_COLOR_VERSION}`
  const hiddenSlot = AUTOMATIC_COLOR_SLOTS[1]!
  const saved: [string, number][] = JSON.parse(localStorage.getItem(key)!)
  localStorage.setItem(
    key,
    JSON.stringify([...saved, ['hidden tab owner', hiddenSlot]])
  )
  window.dispatchEvent(new StorageEvent('storage', { key }))
  state.color('new')
  expect(state.savedColorSlots('services').map(([id]) => id)).toContain(
    'hidden tab owner'
  )
  expect(state.color('existing')).toBe(existing)
  expect(state.color('hidden tab owner')).toBe(
    GRAPH_SWATCHES[hiddenSlot]!.color
  )
  expect(state.color('new')).not.toBe(GRAPH_SWATCHES[hiddenSlot]!.color)
})

it('keeps in-memory ownership when browser storage is blocked', async () => {
  const state = await setup()
  const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('blocked')
  })
  try {
    const warm = state.color('service')
    const cool = state.color('service')
    expect(cool).toBe(warm)
    state.color('added')
    expect(state.color('service')).toBe(warm)
    expect(state.color('service')).toBe(cool)
  } finally {
    spy.mockRestore()
  }
})
