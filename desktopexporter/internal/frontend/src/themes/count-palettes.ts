import ramps from './count-palettes.json' with { type: 'json' }
import englishNames from './count-palette-names.json' with { type: 'json' }

export type CountPaletteId = keyof typeof ramps
export const DEFAULT_COUNT_PALETTE: CountPaletteId = 'to-ro'
// The generated ramp keys retain the pinned Madoromi full-ramp colour order.
function isCountPaletteId(id: string): id is CountPaletteId {
  return Object.hasOwn(ramps, id)
}

export const COUNT_PALETTES = Object.keys(ramps).map(id => {
  if (!isCountPaletteId(id)) throw new Error(`Unknown count palette: ${id}`)
  return {
    id,
    name: id[0]!.toUpperCase() + id.slice(1),
    english: englishNames[id],
  }
})

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
