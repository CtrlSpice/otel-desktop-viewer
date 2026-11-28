import palette from './graph-palette.json' with { type: 'json' }

export type GraphInkId = keyof typeof palette.names
export type GraphShade = 1 | 2 | 3 | 4 | 5 | 6 | 7
export type GraphColorId = `${GraphInkId}:${GraphShade}`
export type GraphSwatch = {
  id: GraphColorId
  color: string
  ink: GraphInkId
  shade: GraphShade
}

export const GRAPH_PALETTE_VERSION = palette.version
const shades: readonly GraphShade[] = [1, 2, 3, 4, 5, 6, 7]

function isGraphInkId(id: string): id is GraphInkId {
  return Object.hasOwn(palette.names, id)
}

export const GRAPH_INKS = Object.keys(palette.names).map(id => {
  if (!isGraphInkId(id)) throw new Error(`Unknown graph ink: ${id}`)
  return {
    id,
    name: id[0]!.toUpperCase() + id.slice(1),
    english: palette.names[id],
    swatches: shades.map((shade): GraphSwatch => {
      const color = palette.inks[id][shade - 1]
      if (color === undefined) {
        throw new Error(`Missing graph shade: ${id}:${shade}`)
      }
      return { id: `${id}:${shade}`, color, ink: id, shade }
    }),
  }
})

const swatchesById = new Map<string, GraphSwatch>(
  GRAPH_INKS.flatMap(ink =>
    ink.swatches.map(swatch => [swatch.id, swatch] as const)
  )
)

// Freeze identity slots independently of filtering or visible series.
export const GRAPH_SWATCHES = palette.slots.map(id => swatchesById.get(id)!)
export const GRAPH_COLORS: readonly string[] = GRAPH_SWATCHES.map(
  swatch => swatch.color
)

export function graphSwatch(id: string | undefined): GraphSwatch | undefined {
  if (id === undefined) return undefined
  return swatchesById.get(id)
}
