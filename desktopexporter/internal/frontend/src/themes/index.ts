import definitions from './madoromi.json'
import graphDefinitions from './rose-pine.json'
import seriesPalette from './series-palette.json'

export type ThemeName = keyof typeof definitions
export const DEFAULT_THEME: ThemeName = 'madoromi-fuyu-yoru'
export const SEASONS = [
  { id: 'haru', name: 'Spring' },
  { id: 'natsu', name: 'Summer' },
  { id: 'aki', name: 'Autumn' },
  { id: 'fuyu', name: 'Winter' },
] as const
export type Season = (typeof SEASONS)[number]['id']
export type ColorScheme = 'light' | 'dark'

export const SERIES_PALETTE_VERSION = seriesPalette.version
export const SERIES_COLOR_COUNT = seriesPalette.slots.length
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
const legacy = new Map<string, ThemeName>([
  ['rose-pine-dawn', 'madoromi-fuyu-hiru'],
  ['rose-pine-moon', 'madoromi-fuyu-yoru'],
  ['rose-pine', 'madoromi-fuyu-yoru'],
  ['madoromi-day', 'madoromi-fuyu-hiru'],
  ['madoromi-night', 'madoromi-fuyu-yoru'],
])

/** Unknown names use Winter Night; previous theme IDs retain their mode. */
export function getTheme(name: string = ''): Theme {
  return byName.get(legacy.get(name) ?? name) ?? byName.get(DEFAULT_THEME)!
}

/** Retained chart palettes during the interface-only Madoromi migration. */
export const GRAPH_THEMES = (
  ['rose-pine-dawn', 'rose-pine-moon', 'rose-pine'] as const
).map(id => ({ id, ...graphDefinitions[id] }))
const graphByName = new Map(
  GRAPH_THEMES.map(theme => [theme.id as string, theme])
)

export function getGraphTheme(name: string = '') {
  return (
    graphByName.get(name) ??
    graphByName.get(
      getTheme(name).scheme === 'light' ? 'rose-pine-dawn' : 'rose-pine-moon'
    )!
  )
}

/** Same frozen Bloom slots and storage namespace across the new seasons. */
export function getSeriesPalette(name: string = ''): readonly string[] {
  return seriesPalette.themes[getGraphTheme(name).id]
}

export function restoreTheme(
  saved: string | null,
  prefersDark: boolean
): Theme {
  if (saved && (byName.has(saved) || legacy.has(saved))) return getTheme(saved)
  return getTheme(themeName('fuyu', prefersDark ? 'dark' : 'light'))
}
