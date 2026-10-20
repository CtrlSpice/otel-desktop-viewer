import { PANEL_DEFAULT_REM, PANEL_MIN_REM, remToPx } from './panel-width'

const STORAGE_KEY = 'signal-detail-width'
export const DETAIL_SPLIT_STORAGE_KEY = 'workspace-panel-split'

// The layout limits the rendered width to the available space, not a fixed cap.
function clampDetailRem(rem: number): number {
  return Number.isFinite(rem) ? Math.max(PANEL_MIN_REM, rem) : PANEL_DEFAULT_REM
}

function readNumber(key: string): number | null {
  try {
    const raw = localStorage.getItem(key)
    if (raw === null || raw.trim() === '') return null
    const value = Number(raw)
    return Number.isFinite(value) ? value : null
  } catch {
    return null
  }
}

function writeNumber(key: string, value: number) {
  try {
    localStorage.setItem(key, String(value))
  } catch {
    // A full or blocked store costs persistence, not resizing.
  }
}

// A workspace preference: every route uses the same detail width.
const savedWidth = readNumber(STORAGE_KEY)
let widthRem = $state(clampDetailRem(savedWidth ?? PANEL_DEFAULT_REM))
let hasWidthPreference = savedWidth !== null
let migratedStackedSplit = false

export const detailWidth = {
  get rem() {
    return widthRem
  },
  set(rem: number) {
    widthRem = clampDetailRem(rem)
    hasWidthPreference = true
    writeNumber(STORAGE_KEY, widthRem)
  },
  migrateLegacy(path: string, flexSpacePx: number, stacked: boolean) {
    if (!Number.isFinite(flexSpacePx) || flexSpacePx <= 0) return
    const page = path.split('/')[1]
    const key =
      page === 'traces'
        ? 'trace-detail-panels'
        : page === 'metrics'
          ? 'metric-detail-panels'
          : null
    if (!key) return
    const legacy = readNumber(key)
    if (legacy === null) return
    // Legacy values are main-pane flex fractions, not pixels or rem.
    const left = Math.max(0, Math.min(1, legacy))
    if (stacked) {
      // A height fraction cannot initialize the shared horizontal width.
      if (
        !migratedStackedSplit &&
        readNumber(DETAIL_SPLIT_STORAGE_KEY) === null
      ) {
        writeNumber(DETAIL_SPLIT_STORAGE_KEY, left)
        migratedStackedSplit = true
      }
      return
    }
    if (hasWidthPreference) return
    // Recheck storage in case another consumer saved after module initialization.
    const saved = readNumber(STORAGE_KEY)
    this.set(saved ?? ((1 - left) * flexSpacePx) / remToPx(1))
  },
}
