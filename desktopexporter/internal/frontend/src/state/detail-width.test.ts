// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PANEL_DEFAULT_REM, PANEL_MIN_REM } from './panel-width'

const KEY = 'signal-detail-width'

async function freshStore() {
  vi.resetModules()
  return import('./detail-width.svelte')
}

describe('app-wide detail width', () => {
  beforeEach(() => localStorage.clear())

  it('starts at the shared default', async () => {
    const { detailWidth } = await freshStore()
    expect(detailWidth.rem).toBe(PANEL_DEFAULT_REM)
  })

  it('persists widths above the old cap for all consumers and restores them after reload', async () => {
    const { detailWidth } = await freshStore()
    detailWidth.set(60)
    const otherConsumer = await import('./detail-width.svelte')
    expect(otherConsumer.detailWidth.rem).toBe(60)
    expect(localStorage.getItem(KEY)).toBe('60')
    expect((await freshStore()).detailWidth.rem).toBe(60)
  })

  it.each([
    ['wide', PANEL_DEFAULT_REM],
    ['900', 900],
    ['Infinity', PANEL_DEFAULT_REM],
    ['40rem', PANEL_DEFAULT_REM],
    ['', PANEL_DEFAULT_REM],
    ['-1', PANEL_MIN_REM],
  ])('handles stored width %s', async (stored, expected) => {
    localStorage.setItem(KEY, stored)
    expect((await freshStore()).detailWidth.rem).toBe(expected)
  })

  it('keeps the live width when storage is unavailable', async () => {
    const { detailWidth } = await freshStore()
    const write = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new DOMException('Storage full', 'QuotaExceededError')
      })
    try {
      detailWidth.set(32)
      expect(detailWidth.rem).toBe(32)
    } finally {
      write.mockRestore()
    }
  })

  it.each([
    ['/traces/trace-id', 'trace-detail-panels'],
    ['/metrics/metric-id', 'metric-detail-panels'],
  ])(
    'migrates the active %s split once, using usable space and the root font size',
    async (path, key) => {
      localStorage.setItem('trace-detail-panels', '0.8')
      localStorage.setItem('metric-detail-panels', '0.8')
      localStorage.setItem(key, '0.4')
      document.documentElement.style.fontSize = '20px'
      try {
        const { detailWidth } = await freshStore()
        detailWidth.migrateLegacy(path, 0, false)
        expect(localStorage.getItem(KEY)).toBeNull()
        detailWidth.migrateLegacy(path, 1800, false)
        expect(detailWidth.rem).toBe(54)
        expect(localStorage.getItem(KEY)).toBe('54')
        detailWidth.migrateLegacy('/metrics/another', 1000, false)
        expect(detailWidth.rem).toBe(54)
        expect((await freshStore()).detailWidth.rem).toBe(54)
        expect(localStorage.getItem(key)).toBe('0.4')
      } finally {
        document.documentElement.style.fontSize = ''
      }
    }
  )

  it('never replaces a valid shared width, including one saved after initialization', async () => {
    localStorage.setItem('trace-detail-panels', '0.4')
    const { detailWidth } = await freshStore()
    localStorage.setItem(KEY, '60')
    detailWidth.migrateLegacy('/traces', 1800, false)
    expect(detailWidth.rem).toBe(60)
    const reloaded = (await freshStore()).detailWidth
    reloaded.migrateLegacy('/traces', 1200, false)
    expect(reloaded.rem).toBe(60)
  })

  it('does not let a legacy split replace a live resize when persistence fails', async () => {
    localStorage.setItem('trace-detail-panels', '0.4')
    const { detailWidth } = await freshStore()
    const write = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new DOMException('Storage full', 'QuotaExceededError')
      })
    try {
      detailWidth.set(60)
      detailWidth.migrateLegacy('/traces', 1200, false)
      expect(detailWidth.rem).toBe(60)
    } finally {
      write.mockRestore()
    }
  })

  it.each(['oops', '0.4oops', '', 'Infinity', 'NaN'])(
    'ignores malformed legacy split %s',
    async value => {
      localStorage.setItem('trace-detail-panels', value)
      const { detailWidth } = await freshStore()
      detailWidth.migrateLegacy('/traces', 1800, false)
      expect(detailWidth.rem).toBe(PANEL_DEFAULT_REM)
      expect(localStorage.getItem(KEY)).toBeNull()
    }
  )

  it('can migrate past a malformed shared preference but ignores unrelated routes', async () => {
    localStorage.setItem(KEY, 'broken')
    localStorage.setItem('trace-detail-panels', '0.4')
    const { detailWidth } = await freshStore()
    detailWidth.migrateLegacy('/logs', 1600, false)
    detailWidth.migrateLegacy('/traces-other', 1600, false)
    expect(detailWidth.rem).toBe(PANEL_DEFAULT_REM)
    detailWidth.migrateLegacy('/traces', 1600, false)
    expect(detailWidth.rem).toBe(60)
  })

  it.each([-2, 2])(
    'keeps an out-of-range legacy fraction %s finite and above the width floor',
    async fraction => {
      localStorage.setItem('trace-detail-panels', String(fraction))
      const { detailWidth } = await freshStore()
      detailWidth.migrateLegacy('/traces', 1600, false)
      expect(detailWidth.rem).toBe(fraction < 0 ? 100 : PANEL_MIN_REM)
    }
  )

  it.each([null, '0.55'])(
    'preserves stacked fractions separately (existing shared split: %s)',
    async shared => {
      localStorage.setItem('trace-detail-panels', '0.6')
      const { detailWidth, DETAIL_SPLIT_STORAGE_KEY } = await freshStore()
      if (shared !== null)
        localStorage.setItem(DETAIL_SPLIT_STORAGE_KEY, shared)
      detailWidth.migrateLegacy('/traces', 700, true)
      expect(localStorage.getItem(DETAIL_SPLIT_STORAGE_KEY)).toBe(
        shared ?? '0.6'
      )
      expect(localStorage.getItem(KEY)).toBeNull()
      detailWidth.migrateLegacy('/traces', 1800, false)
      expect(detailWidth.rem).toBe(45)
    }
  )

  it('loads and attempts migration safely when reads are blocked', async () => {
    const read = vi
      .spyOn(Storage.prototype, 'getItem')
      .mockImplementation(() => {
        throw new DOMException('Storage blocked', 'SecurityError')
      })
    try {
      const { detailWidth } = await freshStore()
      detailWidth.migrateLegacy('/traces', 1800, false)
      expect(detailWidth.rem).toBe(PANEL_DEFAULT_REM)
    } finally {
      read.mockRestore()
    }
  })
})
