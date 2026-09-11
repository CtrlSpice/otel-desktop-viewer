/**
 * How wide the signal list drawer is when open, in rem, remembered per browser.
 *
 * @remarks
 * A browser preference, not telemetry state: it describes the person's screen
 * rather than the data, so it lives in localStorage and stays out of the URL
 * and the store -- the same line `docs/snapshot-save-share.md` draws for theme
 * and panel widths.
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

/**
 * The shared panel rules under the drawer's own names. The bounds are wider
 * above the default than below it (22 / 28 / 40) on purpose: #345 was a
 * complaint that the list could not get *wider*, so that is where the room
 * is. The floor is inherited from the detail pane's tab strip -- stricter
 * than the list itself needs, which beats the 14rem this store first
 * shipped with, a width that truncated the service name and duration.
 * Derivations live in `panel-width.ts`.
 */
export const DEFAULT_DRAWER_WIDTH_REM = PANEL_DEFAULT_REM
export const MIN_DRAWER_WIDTH_REM = PANEL_MIN_REM
export const MAX_DRAWER_WIDTH_REM = PANEL_MAX_REM

const STORAGE_KEY = 'signal-drawer-width'

function load(): number {
  if (typeof localStorage === 'undefined') return DEFAULT_DRAWER_WIDTH_REM
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw === null) return DEFAULT_DRAWER_WIDTH_REM
  const parsed = Number.parseFloat(raw)
  // A stored value can be anything -- hand-edited, or written by a build with
  // different bounds -- so it is clamped rather than trusted, and a
  // non-number falls back rather than rendering a drawer of width NaN.
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
