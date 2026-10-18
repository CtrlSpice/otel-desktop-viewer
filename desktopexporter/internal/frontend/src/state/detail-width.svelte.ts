import { PANEL_DEFAULT_REM, clampPanelRem } from './panel-width'

const STORAGE_KEY = 'signal-detail-width'

function load(): number {
  if (typeof localStorage === 'undefined') return PANEL_DEFAULT_REM
  const raw = localStorage.getItem(STORAGE_KEY)
  return raw === null
    ? PANEL_DEFAULT_REM
    : clampPanelRem(Number.parseFloat(raw))
}

// A workspace preference: every route uses the same detail width.
let widthRem = $state(load())

export const detailWidth = {
  get rem() {
    return widthRem
  },
  set(rem: number) {
    widthRem = clampPanelRem(rem)
    if (typeof localStorage === 'undefined') return
    try {
      localStorage.setItem(STORAGE_KEY, String(widthRem))
    } catch {
      // A full or blocked store costs persistence, not resizing.
    }
  },
}
