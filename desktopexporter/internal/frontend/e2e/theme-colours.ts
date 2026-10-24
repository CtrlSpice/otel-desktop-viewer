import { getTheme, THEMES, type Theme } from '../src/themes'

export { getTheme, THEMES }

export function themeRgb(name: string, role: keyof Theme['colors']): string {
  const hex = getTheme(name).colors[role]
  return `rgb(${[1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16)).join(', ')})`
}
