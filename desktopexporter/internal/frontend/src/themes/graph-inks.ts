import data from './graph-inks.json' with { type: 'json' }
import curves from './graph-ink-curves.json' with { type: 'json' }
import { COUNT_PALETTES, type CountPaletteId } from './count-palettes'

export type GraphShade = 1 | 2 | 3 | 4 | 5 | 6 | 7
export type GraphColorId =
  `${CountPaletteId}:${GraphShade}` | `${CountPaletteId}:curve:${number}`
export type GraphSwatch = {
  id: GraphColorId
  color: string
  ink: CountPaletteId
  position: number
  shade?: GraphShade
}

export const GRAPH_PALETTE_VERSION = data.version
export const GRAPH_CURVE_STEPS = curves.steps
const shades: readonly GraphShade[] = [1, 2, 3, 4, 5, 6, 7]
export const GRAPH_INKS = COUNT_PALETTES.map(ink => ({
  ...ink,
  swatches: shades.map((shade): GraphSwatch => {
    const color = data.inks[ink.id][shade - 1]
    const position = curves.inks[ink.id].positions[shade - 1]
    if (color === undefined || position === undefined)
      throw new Error(`Missing graph shade: ${ink.id}:${shade}`)
    return { id: `${ink.id}:${shade}`, color, ink: ink.id, position, shade }
  }),
}))
const swatchesById = new Map<string, GraphSwatch>(
  GRAPH_INKS.flatMap(ink =>
    ink.swatches.map(swatch => [swatch.id, swatch] as const)
  )
)
// Freeze identity slots independently of filtering or visible series.
export const GRAPH_SWATCHES = data.slots.map(id => swatchesById.get(id)!)
export const GRAPH_COLORS: readonly string[] = GRAPH_SWATCHES.map(
  swatch => swatch.color
)
const inksById = new Map<string, (typeof GRAPH_INKS)[number]>(
  GRAPH_INKS.map(ink => [ink.id, ink])
)
const curveCache = new Map<string, readonly GraphSwatch[]>()

export function graphInkCurve(id: string): readonly GraphSwatch[] {
  const cached = curveCache.get(id)
  if (cached) return cached
  const ink = inksById.get(id)
  if (!ink) return []
  const { colors } = curves.inks[ink.id]
  const values = Array.from(
    { length: GRAPH_CURVE_STEPS + 1 },
    (_, position): GraphSwatch => ({
      id: `${ink.id}:curve:${position}`,
      ink: ink.id,
      position,
      color: `#${colors.slice(position * 6, position * 6 + 6)}`,
    })
  )
  curveCache.set(id, values)
  return values
}

export function graphSwatch(id: string | undefined): GraphSwatch | undefined {
  if (id === undefined) return undefined
  const preset = swatchesById.get(id)
  if (preset) return preset
  const match = /^([^:]+):curve:(0|[1-9]\d*)$/.exec(id)
  if (!match || Number(match[2]) > GRAPH_CURVE_STEPS) return undefined
  return graphInkCurve(match[1]!)[Number(match[2])]
}
