import definitions from './madoromi.json'
import { GRAPH_COLORS, GRAPH_PALETTE_VERSION } from './graph-inks'

export type ThemeName = keyof typeof definitions
export const DEFAULT_THEME: ThemeName = 'madoromi-aki-yoru'
export const SEASONS = [
  { id: 'haru', name: 'Spring' },
  { id: 'natsu', name: 'Summer' },
  { id: 'aki', name: 'Autumn' },
  { id: 'fuyu', name: 'Winter' },
] as const
export type Season = (typeof SEASONS)[number]['id']
export type ColorScheme = 'light' | 'dark'

export const SERIES_PALETTE_VERSION = GRAPH_PALETTE_VERSION
export const SERIES_COLOR_COUNT = GRAPH_COLORS.length
export function themeName(season: Season, scheme: ColorScheme): ThemeName {
  return `madoromi-${season}-${scheme === 'light' ? 'hiru' : 'yoru'}`
}
export const THEMES = SEASONS.flatMap(season =>
  (['light', 'dark'] as const).map(scheme => {
    const id = themeName(season.id, scheme)
    return { id, season: season.id, scheme, ...definitions[id] }
  })
)
export type Theme = (typeof THEMES)[number]

const byName = new Map<string, Theme>(THEMES.map(theme => [theme.id, theme]))
/** Unknown names use Autumn Night. */
export function getTheme(name: string = ''): Theme {
  return byName.get(name) ?? byName.get(DEFAULT_THEME)!
}

/** Official ink shades retain their identity across seasons and Day/Night. */
export function getSeriesPalette(_name: string = ''): readonly string[] {
  return GRAPH_COLORS
}

export function restoreTheme(
  saved: string | null,
  prefersDark: boolean
): Theme {
  if (saved && byName.has(saved)) return getTheme(saved)
  return getTheme(themeName('aki', prefersDark ? 'dark' : 'light'))
}
