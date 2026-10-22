import definitions from './rose-pine.json'
import seriesPalette from './series-palette.json'

export type ThemeName = keyof typeof definitions
export const DEFAULT_THEME: ThemeName = 'rose-pine-moon'

const ORDER = ['rose-pine-dawn', 'rose-pine-moon', 'rose-pine'] as const
export const METRIC_SERIES = Object.values(seriesPalette.series)
const metricOnly = new Set<string>(seriesPalette.metricOnly)
export const TRACE_SERIES = Object.entries(seriesPalette.series)
  .filter(([name]) => !metricOnly.has(name))
  .map(([, color]) => color)
// Trace identities exclude error-like pinks/mauves and fault-like orange.
TRACE_SERIES.splice(4, 0, ...Object.values(seriesPalette.traceOnly))
export const THEMES = ORDER.map(id => ({
  id,
  ...definitions[id],
}))
export type Theme = (typeof THEMES)[number]

const byName = new Map<string, Theme>(THEMES.map(theme => [theme.id, theme]))

/** Unknown names use Moon, including inherited object-property names. */
export function getTheme(name: string = ''): Theme {
  return byName.get(name) ?? byName.get(DEFAULT_THEME)!
}

export function restoreTheme(
  saved: string | null,
  prefersDark: boolean
): Theme {
  if (saved && byName.has(saved)) return getTheme(saved)
  if (saved === 'madoromi-day') return getTheme('rose-pine-dawn')
  if (saved === 'madoromi-night') return getTheme('rose-pine-moon')
  return getTheme(prefersDark ? 'rose-pine-moon' : 'rose-pine-dawn')
}
