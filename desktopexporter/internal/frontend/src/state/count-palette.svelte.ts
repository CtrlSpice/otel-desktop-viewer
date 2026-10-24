import {
  DEFAULT_COUNT_PALETTE,
  restoreCountPalette,
  type CountPaletteId,
} from '@/themes/count-palettes'

const STORAGE_KEY = 'graph-count-palette'

function readSavedPalette(): CountPaletteId {
  try {
    return restoreCountPalette(localStorage.getItem(STORAGE_KEY))
  } catch {
    return DEFAULT_COUNT_PALETTE
  }
}

let value = $state<CountPaletteId>(readSavedPalette())

export const countPalette = {
  get value() {
    return value
  },
  set(next: CountPaletteId) {
    value = next
    try {
      localStorage.setItem(STORAGE_KEY, value)
    } catch {
      // A full or blocked store costs persistence, not palette interaction.
    }
  },
}
