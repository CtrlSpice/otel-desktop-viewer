// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PANEL_DEFAULT_REM, PANEL_MIN_REM, PANEL_MAX_REM } from './panel-width'

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

  it('persists one width for all consumers and restores it after reload', async () => {
    const { detailWidth } = await freshStore()
    detailWidth.set(32)
    const otherConsumer = await import('./detail-width.svelte')
    expect(otherConsumer.detailWidth.rem).toBe(32)
    expect(localStorage.getItem(KEY)).toBe('32')
    expect((await freshStore()).detailWidth.rem).toBe(32)
  })

  it.each([
    ['wide', PANEL_DEFAULT_REM],
    ['900', PANEL_MAX_REM],
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
})
