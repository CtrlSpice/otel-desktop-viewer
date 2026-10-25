import { getTheme, THEMES, type Theme } from '../src/themes'

export { getTheme, THEMES }

/** Resolve the fixed interface accents on the seasonal paper. */
export function themeRgb(name: string, role: keyof Theme['colors']): string {
  const theme = getTheme(name)
  const hex = theme.colors[role]
  return `rgb(${[1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16)).join(', ')})`
}
