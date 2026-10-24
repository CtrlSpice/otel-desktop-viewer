import data from './graph-inks.json'
import { COUNT_PALETTES, type CountPaletteId } from './count-palettes'

export type GraphShade = 1 | 2 | 3 | 4 | 5 | 6 | 7
export type GraphColorId = `${CountPaletteId}:${GraphShade}`
export type GraphSwatch = { id: GraphColorId; color: string; shade: GraphShade }

export const GRAPH_PALETTE_VERSION = data.version
export const LEGACY_GRAPH_PALETTE = data.legacy
export const GRAPH_INKS = COUNT_PALETTES.map(ink => ({
  ...ink,
  swatches: data.inks[ink.id].slice(0, 7).map((color, index): GraphSwatch => ({
    id: `${ink.id}:${index + 1}` as GraphColorId,
    color,
    shade: (index + 1) as GraphShade,
  })),
}))
const swatchesById = new Map<string, GraphSwatch>(
  GRAPH_INKS.flatMap(ink =>
    ink.swatches.map(swatch => [swatch.id, swatch] as const)
  )
)
// Freeze identity slots independently of picker filtering or visible series.
export const GRAPH_SWATCHES = data.slots.map(id => swatchesById.get(id)!)
export const GRAPH_COLORS: readonly string[] = GRAPH_SWATCHES.map(
  swatch => swatch.color
)

export function graphSwatch(id: string | undefined): GraphSwatch | undefined {
  return id === undefined ? undefined : swatchesById.get(id)
}
