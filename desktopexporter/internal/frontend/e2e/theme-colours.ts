import { getTheme, THEMES, type Theme } from '../src/themes'
import {
  accentInkColors,
  DEFAULT_ACCENT_INK,
  type AccentInkId,
} from '../src/themes/accent-inks'

export { getTheme, THEMES }

/** Resolve the displayed palette, including the independently selected ink. */
export function themeRgb(
  name: string,
  role: keyof Theme['colors'],
  ink: AccentInkId = DEFAULT_ACCENT_INK
): string {
  const theme = getTheme(name)
  const colors = { ...theme.colors, ...accentInkColors(ink, theme.id).colors }
  const hex = colors[role]
  return `rgb(${[1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16)).join(', ')})`
}
