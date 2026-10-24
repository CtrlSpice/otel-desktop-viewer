import {
  restoreAccentInk,
  DEFAULT_ACCENT_INK,
  type AccentInkId,
} from '@/themes/accent-inks'
import { countPalette } from './count-palette.svelte'

const STORAGE_KEY = 'theme-accent-ink'

function readSaved(): AccentInkId {
  try {
    return restoreAccentInk(localStorage.getItem(STORAGE_KEY))
  } catch {
    return DEFAULT_ACCENT_INK
  }
}

const initial = readSaved()
let value = $state<AccentInkId>(initial)

function apply(next: AccentInkId) {
  value = next
  document.documentElement.dataset.accentInk = next
}

export const accentInk = {
  get value() {
    return value
  },
  set(next: AccentInkId) {
    apply(next)
    countPalette.set(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // Keep the chosen ink in memory when storage is unavailable.
    }
  },
}

if (typeof window !== 'undefined') {
  apply(initial)
  window.addEventListener('storage', event => {
    if (event.key === null || event.key === STORAGE_KEY) apply(readSaved())
  })
}
