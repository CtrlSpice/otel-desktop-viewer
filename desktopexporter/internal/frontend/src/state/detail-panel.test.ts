// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'

const KEY = 'signal-detail:open'

async function freshStore() {
  vi.resetModules()
  return (await import('./detail-panel.svelte')).detailPanel
}

describe('detail panel visibility', () => {
  beforeEach(() => localStorage.clear())

  it('restores the stored collapsed state', async () => {
    localStorage.setItem(KEY, 'false')
    expect((await freshStore()).open).toBe(false)
  })

  it('reveals repeated selections and persists the open state', async () => {
    const panel = await freshStore()
    expect(panel.open).toBe(true)
    panel.setOpen(false)
    expect(localStorage.getItem(KEY)).toBe('false')
    panel.reveal()
    expect(panel.open).toBe(true)
    expect(panel.revealRequest).toBe(1)
    expect(localStorage.getItem(KEY)).toBe('true')
    panel.reveal()
    expect(panel.revealRequest).toBe(2)
    expect((await freshStore()).open).toBe(true)
  })

  it('does not request a selection when toggled manually', async () => {
    const panel = await freshStore()
    panel.setOpen(false)
    panel.setOpen(true)
    expect(panel.revealRequest).toBe(0)
  })

  it('still reveals the panel when persistence fails', async () => {
    const panel = await freshStore()
    panel.setOpen(false)
    const write = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new DOMException('Storage full', 'QuotaExceededError')
      })
    try {
      panel.reveal()
      expect(panel.open).toBe(true)
      expect(panel.revealRequest).toBe(1)
    } finally {
      write.mockRestore()
    }
  })
})
