// @vitest-environment jsdom
import { describe, expect, it, beforeEach, vi } from 'vitest'

const KEY = 'signal-drawer-width'

async function freshStore() {
  // The width is module state seeded from localStorage at import, so each
  // case needs its own module instance to exercise loading. resetModules
  // rather than a cache-busting query: a query string on a .svelte.ts
  // specifier does not survive the Svelte transform.
  vi.resetModules()
  return import('./drawer-width.svelte')
}

describe('drawer width', () => {
  beforeEach(() => localStorage.clear())

  it('shares the panel-wide default and bounds', async () => {
    // One set of rules for every side panel; the drawer only renames them.
    const {
      DEFAULT_DRAWER_WIDTH_REM,
      MIN_DRAWER_WIDTH_REM,
      MAX_DRAWER_WIDTH_REM,
    } = await freshStore()
    const { PANEL_DEFAULT_REM, PANEL_MIN_REM, PANEL_MAX_REM } = await import(
      './panel-width'
    )
    expect(DEFAULT_DRAWER_WIDTH_REM).toBe(PANEL_DEFAULT_REM)
    expect(MIN_DRAWER_WIDTH_REM).toBe(PANEL_MIN_REM)
    expect(MAX_DRAWER_WIDTH_REM).toBe(PANEL_MAX_REM)
  })

  it('starts at the default with nothing stored', async () => {
    const { drawerWidth, DEFAULT_DRAWER_WIDTH_REM } = await freshStore()
    expect(drawerWidth.rem).toBe(DEFAULT_DRAWER_WIDTH_REM)
  })

  it('clamps a drag past either bound', async () => {
    const { drawerWidth, MIN_DRAWER_WIDTH_REM, MAX_DRAWER_WIDTH_REM } =
      await freshStore()
    drawerWidth.preview(1000)
    expect(drawerWidth.rem).toBe(MAX_DRAWER_WIDTH_REM)
    drawerWidth.preview(-50)
    expect(drawerWidth.rem).toBe(MIN_DRAWER_WIDTH_REM)
  })

  it('persists only on commit, so a drag does not write per pointermove', async () => {
    const { drawerWidth } = await freshStore()
    drawerWidth.preview(30)
    expect(localStorage.getItem(KEY)).toBeNull()
    drawerWidth.commit()
    expect(localStorage.getItem(KEY)).toBe('30')
  })

  it('restores a stored width', async () => {
    localStorage.setItem(KEY, '33')
    const { drawerWidth } = await freshStore()
    expect(drawerWidth.rem).toBe(33)
  })

  it('clamps a stored width that is out of bounds', async () => {
    // Hand-edited, or written by a build with different bounds.
    localStorage.setItem(KEY, '900')
    const { drawerWidth, MAX_DRAWER_WIDTH_REM } = await freshStore()
    expect(drawerWidth.rem).toBe(MAX_DRAWER_WIDTH_REM)
  })

  it('falls back when the stored value is not a number', async () => {
    // Rendering a drawer of width NaN is worse than ignoring the entry.
    localStorage.setItem(KEY, 'wide please')
    const { drawerWidth, DEFAULT_DRAWER_WIDTH_REM } = await freshStore()
    expect(drawerWidth.rem).toBe(DEFAULT_DRAWER_WIDTH_REM)
  })

  it('resets to the default and persists it', async () => {
    localStorage.setItem(KEY, '24')
    const { drawerWidth, DEFAULT_DRAWER_WIDTH_REM } = await freshStore()
    drawerWidth.reset()
    expect(drawerWidth.rem).toBe(DEFAULT_DRAWER_WIDTH_REM)
    expect(localStorage.getItem(KEY)).toBe(String(DEFAULT_DRAWER_WIDTH_REM))
  })
})
