import englishNames from './count-palette-names.json' with { type: 'json' }

export type CountPaletteId = keyof typeof englishNames
// The generated catalogue retains the pinned Madoromi full-ramp colour order.
function isCountPaletteId(id: string): id is CountPaletteId {
  return Object.hasOwn(englishNames, id)
}

export const COUNT_PALETTES = Object.keys(englishNames).map(id => {
  if (!isCountPaletteId(id)) throw new Error(`Unknown count palette: ${id}`)
  return {
    id,
    name: id[0]!.toUpperCase() + id.slice(1),
    english: englishNames[id],
  }
})
