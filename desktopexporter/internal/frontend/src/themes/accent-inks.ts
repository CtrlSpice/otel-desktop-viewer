import data from './accent-inks.json'
import { COUNT_PALETTES, type CountPaletteId } from './count-palettes'
import type { ThemeName } from './index'

export type AccentInkId = CountPaletteId
export const ACCENT_INKS = COUNT_PALETTES
export const DEFAULT_ACCENT_INK = data.default as AccentInkId

export function restoreAccentInk(value: string | null): AccentInkId {
  return ACCENT_INKS.find(ink => ink.id === value)?.id ?? DEFAULT_ACCENT_INK
}

export function accentInkColors(ink: AccentInkId, theme: ThemeName) {
  return data.inks[ink].themes[theme]
}

/** Use the actual mixed role colours, rather than the seed's graph swatches. */
export function accentInkPreview(ink: AccentInkId, theme: ThemeName): string[] {
  const { syntax } = accentInkColors(ink, theme)
  return [
    syntax.ai,
    syntax.ruri,
    syntax.sora,
    syntax.fuji,
    syntax.kikyo,
    syntax.botan,
    syntax.nadeshiko,
  ]
}
