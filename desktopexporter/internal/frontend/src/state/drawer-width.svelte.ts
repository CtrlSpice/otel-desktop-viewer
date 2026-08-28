/**
 * How wide the signal list drawer is when open, in rem, remembered per browser.
 *
 * @remarks
 * A browser preference, not telemetry state: it describes the person's screen
 * rather than the data, so it lives in localStorage and stays out of the URL
 * and the store -- the same line `notes/saved-views.md` draws for theme and
 * panel widths.
 *
 * Rem rather than pixels so the drawer keeps its proportion to the text it
 * holds when the root font size changes.
 */

/** Matches the width the drawer had before it could be resized. */
export const DEFAULT_DRAWER_WIDTH_REM = 28

/**
 * Half again either side of the default, rather than two chosen numbers.
 *
 * Anchoring the bounds to the default keeps them meaningful if it ever
 * changes, and half is enough range to matter: narrow enough to hand the
 * waterfall real space on a laptop, wide enough on a big display to stop
 * trace names, services and durations competing for the same inches --
 * which is the complaint (#345).
 */
export const MIN_DRAWER_WIDTH_REM = DEFAULT_DRAWER_WIDTH_REM * 0.5
export const MAX_DRAWER_WIDTH_REM = DEFAULT_DRAWER_WIDTH_REM * 1.5

const STORAGE_KEY = 'signal-drawer-width'

function clamp(rem: number): number {
  return Math.min(MAX_DRAWER_WIDTH_REM, Math.max(MIN_DRAWER_WIDTH_REM, rem))
}

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
