import data from './accent-inks.json' with { type: 'json' }
import type { CountPaletteId } from './count-palettes'
import { GRAPH_INKS } from './graph-inks'
import type { ThemeName } from './index'

export type AccentInkId = CountPaletteId
// A named ink is previewed by its own official shades, not a selection of
// syntax roles that can come entirely from neighbouring inks in its mix.
export const ACCENT_INKS = GRAPH_INKS
export const DEFAULT_ACCENT_INK = data.default as AccentInkId

export function restoreAccentInk(value: string | null): AccentInkId {
  return ACCENT_INKS.find(ink => ink.id === value)?.id ?? DEFAULT_ACCENT_INK
}

export function accentInkColors(ink: AccentInkId, theme: ThemeName) {
  return data.inks[ink].themes[theme]
}
