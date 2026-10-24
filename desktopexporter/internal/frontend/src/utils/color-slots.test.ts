// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getSeriesPalette,
  SERIES_COLOR_COUNT,
  SERIES_PALETTE_VERSION,
  THEMES,
} from '@/themes'
import { ColorSlotAllocator, colorIdentityHash } from './color-slots'

const SIZE = SERIES_COLOR_COUNT

describe('colour-slot allocation', () => {
  it('starts at the stable hash and probes forward without moving owners', () => {
    const start = colorIdentityHash('new-service') % SIZE
    const slots = new ColorSlotAllocator(SIZE, [['existing-service', start]])
    expect(slots.assign('new-service')).toBe((start + 1) % SIZE)
    expect(slots.assign('existing-service')).toBe(start)
    expect(slots.assign('new-service')).toBe((start + 1) % SIZE)
    expect(slots.reuses()).toEqual([])
  })

  it('fills every slot before reuse and preserves every previous assignment', () => {
    const slots = new ColorSlotAllocator(SIZE)
    for (let i = 0; i < SIZE; i++) slots.assign(`service-${i}`)
    const before = slots.snapshot()
    expect(new Set(before.map(([, slot]) => slot)).size).toBe(SIZE)
    slots.assign('overflow')
    expect(slots.snapshot().slice(0, SIZE)).toEqual(before)
    expect(slots.reuses()).toHaveLength(1)
    expect(slots.reuses()[0]!.identities).toContain('overflow')
  })

  it('restores saved assignments independently of subsequent encounter order', () => {
    const first = new ColorSlotAllocator(SIZE)
    first.assign('api-1')
    first.assign('api-2')
    const restored = new ColorSlotAllocator(SIZE, first.snapshot())
    expect(restored.assign('api-2')).toBe(first.assign('api-2'))
    expect(restored.assign('api-1')).toBe(first.assign('api-1'))
  })

  it('keeps exact identities, including case, whitespace and Unicode', () => {
    const slots = new ColorSlotAllocator(SIZE)
    const identities = ['api', 'API', 'api ', '', '服務', '__proto__']
    for (const identity of identities) slots.assign(identity)
    expect(slots.snapshot().map(([identity]) => identity)).toEqual(identities)
    expect(new Set(slots.snapshot().map(([, slot]) => slot)).size).toBe(
      identities.length
    )
    expect(colorIdentityHash('api')).not.toBe(colorIdentityHash('api '))
  })

  it('uses theme-independent logical slots, not stored hex colours', () => {
    const slots = new ColorSlotAllocator(SIZE)
    const slot = slots.assign('worker')
    for (const theme of THEMES) {
      expect(getSeriesPalette(theme.id)[slot]).toMatch(/^#[0-9a-f]{6}$/)
      expect(slots.assign('worker')).toBe(slot)
    }
  })

  it('rejects invalid palette sizes', () => {
    for (const size of [0, -1, 2.5, Infinity, NaN]) {
      expect(() => new ColorSlotAllocator(size)).toThrow(RangeError)
    }
  })
})

describe('saved colour slots', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  it('survives module reloads and separates services from metric scopes', async () => {
    const first = await import('./color-slots')
    const slot = first.identityColorSlot('worker', 'services')
    first.identityColorSlot('worker', 'metric:one')
    first.identityColorSlot('worker', 'metric:two')
    expect(localStorage.length).toBe(3)
    vi.resetModules()
    const restored = await import('./color-slots')
    expect(restored.identityColorSlot('worker', 'services')).toBe(slot)
    expect(restored.colorSlotReuses('services')).toEqual([])
  })

  it('validates saved entries and preserves valid slots', async () => {
    const key = `graph-colour-slots:${SERIES_PALETTE_VERSION}:${SIZE}:services`
    localStorage.setItem(
      key,
      JSON.stringify([
        ['kept', 7],
        ['negative', -1],
        ['outside', SIZE],
        ['fraction', 1.5],
        ['text', '4'],
        [null, 3],
        ['extra', 2, 3],
        '__proto__',
      ])
    )
    const { identityColorSlot } = await import('./color-slots')
    expect(identityColorSlot('kept', 'services')).toBe(7)
    identityColorSlot('new', 'services')
    expect(JSON.parse(localStorage.getItem(key)!)).toHaveLength(2)
  })

  it('retains in-memory assignments when storage is blocked', async () => {
    const { identityColorSlot } = await import('./color-slots')
    const spy = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new Error('blocked')
      })
    try {
      const slot = identityColorSlot('worker', 'blocked')
      identityColorSlot('other', 'blocked')
      expect(identityColorSlot('worker', 'blocked')).toBe(slot)
    } finally {
      spy.mockRestore()
    }
  })

  it('merges newly saved owners before assigning another identity', async () => {
    const { identityColorSlot } = await import('./color-slots')
    const key = `graph-colour-slots:${SERIES_PALETTE_VERSION}:${SIZE}:services`
    const existing = identityColorSlot('existing', 'services')
    const target = colorIdentityHash('new') % SIZE
    const otherSlot = target === existing ? (target + 1) % SIZE : target
    const saved: [string, number][] = JSON.parse(localStorage.getItem(key)!)
    saved.push(['another-tab', otherSlot])
    localStorage.setItem(key, JSON.stringify(saved))
    const next = identityColorSlot('new', 'services')
    expect(next).not.toBe(existing)
    expect(next).not.toBe(otherSlot)
    expect(identityColorSlot('existing', 'services')).toBe(existing)
    expect(JSON.parse(localStorage.getItem(key)!)).toContainEqual([
      'another-tab',
      otherSlot,
    ])
  })

  it('resolves the previously colliding evaluation services consistently', async () => {
    const { traceServiceColor } = await import('./trace-service-color')
    const one = traceServiceColor('claude-code-eval', 'rose-pine-moon')
    const two = traceServiceColor('viewer-eval', 'rose-pine-moon')
    expect(one).not.toBe(two)
    expect(traceServiceColor('claude-code-eval', 'rose-pine-moon')).toBe(one)
    for (const theme of THEMES) {
      const slot = getSeriesPalette('rose-pine-moon').indexOf(one)
      expect(traceServiceColor('claude-code-eval', theme.id)).toBe(
        getSeriesPalette(theme.id)[slot]
      )
    }
  })
})
