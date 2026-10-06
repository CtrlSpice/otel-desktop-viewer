/**
 * How wide the signal list drawer is when open, in rem, remembered per browser.
 *
 * @remarks
 * A browser preference, not telemetry state: it describes the person's screen
 * rather than the data, so it lives in localStorage and stays out of the URL
 * and the store.
 *
 * Rem rather than pixels so the drawer keeps its proportion to the text it
 * holds when the root font size changes.
 */

import {
  PANEL_DEFAULT_REM,
  PANEL_MIN_REM,
  PANEL_MAX_REM,
  clampPanelRem as clamp,
} from './panel-width'

/** Shared panel bounds under the drawer's public names. */
export const DEFAULT_DRAWER_WIDTH_REM = PANEL_DEFAULT_REM
export const MIN_DRAWER_WIDTH_REM = PANEL_MIN_REM
export const MAX_DRAWER_WIDTH_REM = PANEL_MAX_REM

const STORAGE_KEY = 'signal-drawer-width'

function load(): number {
  if (typeof localStorage === 'undefined') return DEFAULT_DRAWER_WIDTH_REM
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw === null) return DEFAULT_DRAWER_WIDTH_REM
  const parsed = Number.parseFloat(raw)
  // localStorage is untrusted; reject non-numbers and clamp stale values.
  return Number.isFinite(parsed) ? clamp(parsed) : DEFAULT_DRAWER_WIDTH_REM
}

let widthRem = $state(load())

export const drawerWidth = {
  get rem() {
    return widthRem
  },

  /** Set during a drag: clamped, not yet persisted. */
  preview(rem: number) {
    widthRem = clamp(rem)
  },

  /** Commit the current width, at the end of a drag. */
  commit() {
    if (typeof localStorage === 'undefined') return
    try {
      localStorage.setItem(STORAGE_KEY, String(widthRem))
    } catch {
      // A full or blocked store costs the preference, not the drag.
    }
  },

  /** Back to the width the drawer had before anyone resized it. */
  reset() {
    widthRem = DEFAULT_DRAWER_WIDTH_REM
    this.commit()
  },
}
