import { PANEL_DEFAULT_REM, PANEL_MIN_REM } from './panel-width'

const STORAGE_KEY = 'signal-detail-width'

// The layout limits the rendered width to the available space, not a fixed cap.
function clampDetailRem(rem: number): number {
  return Number.isFinite(rem)
    ? Math.max(PANEL_MIN_REM, rem)
    : PANEL_DEFAULT_REM
}

function load(): number {
  if (typeof localStorage === 'undefined') return PANEL_DEFAULT_REM
  const raw = localStorage.getItem(STORAGE_KEY)
  return raw === null
    ? PANEL_DEFAULT_REM
    : clampDetailRem(Number.parseFloat(raw))
}

// A workspace preference: every route uses the same detail width.
let widthRem = $state(load())

export const detailWidth = {
  get rem() {
    return widthRem
  },
  set(rem: number) {
    widthRem = clampDetailRem(rem)
    if (typeof localStorage === 'undefined') return
    try {
      localStorage.setItem(STORAGE_KEY, String(widthRem))
    } catch {
      // A full or blocked store costs persistence, not resizing.
    }
  },
}
