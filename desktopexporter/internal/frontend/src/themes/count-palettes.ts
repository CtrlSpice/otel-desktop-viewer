import ramps from './count-palettes.json'

export type CountPaletteId = keyof typeof ramps
export const DEFAULT_COUNT_PALETTE: CountPaletteId = 'rikka'
export const COUNT_PALETTES = [
  { id: 'edo-murasaki', name: 'Edo-murasaki' },
  { id: 'rikka', name: 'Rikka' },
  { id: 'ku-jaku', name: 'Ku-jaku' },
  { id: 'shin-ryoku', name: 'Shin-ryoku' },
  { id: 'ina-ho', name: 'Ina-ho' },
  { id: 'to-ro', name: 'To-ro' },
] as const satisfies readonly { id: CountPaletteId; name: string }[]

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
