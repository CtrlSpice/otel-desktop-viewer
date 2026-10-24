import ramps from './count-palettes.json'
import englishNames from './count-palette-names.json'

export type CountPaletteId = keyof typeof ramps
export const DEFAULT_COUNT_PALETTE: CountPaletteId = 'rikka'
// The generated ramp keys are the catalogue, in alphabetical ink-name order.
export const COUNT_PALETTES = (Object.keys(ramps) as CountPaletteId[]).map(id => ({
  id,
  name: id[0]!.toUpperCase() + id.slice(1),
  english: englishNames[id],
}))

export function restoreCountPalette(saved: string | null): CountPaletteId {
  return (
    COUNT_PALETTES.find(palette => palette.id === saved)?.id ??
    DEFAULT_COUNT_PALETTE
  )
}

export const countPaletteRamps: Record<
  CountPaletteId,
  Record<number, { day: string[]; night: string[] }>
> = ramps
